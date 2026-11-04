package messagepack

import (
	"errors"
	"fmt"
	"io"
)

const payloadBufferSize = 32768

// NewWriter attaches a Writer to dst without taking ownership or buffering output.
// A nil interface leaves the Writer unattached until Reset supplies a destination.
func NewWriter(dst io.Writer) (w *Writer) {
	return &Writer{state: &writerState{writer: dst}}
}

// Writer writes typed MessagePack values to a caller-owned writer.
// Its zero value is unattached. Initialized copies share output and failure state.
// I/O failures remain terminal until Reset; shared instances must not be used concurrently.
type Writer struct {
	state   *writerState
	scratch [9]byte
}

// Reset attaches dst and clears the shared failure without flushing or closing.
// It cannot repair a partial message; callers must supply a known value boundary.
func (w *Writer) Reset(dst io.Writer) {
	if w.state == nil {
		w.state = &writerState{}
	}
	w.state.writer = dst
	w.state.err = nil
}

// EncodeNil writes a nil value, preserving the distinction from empty payloads.
func (w *Writer) EncodeNil() (err error) {
	_, err = w.write("encode nil", AppendNil(w.scratch[:0]))
	return err
}

// EncodeBool writes value without buffering output for later operations.
func (w *Writer) EncodeBool(value bool) (err error) {
	_, err = w.write("encode bool", AppendBool(w.scratch[:0], value))
	return err
}

// EncodeInt64 writes the smallest integer representation of value.
// Nonnegative values encode identically to EncodeUint64.
func (w *Writer) EncodeInt64(value int64) (err error) {
	_, err = w.write("encode int64", AppendInt64(w.scratch[:0], value))
	return err
}

// EncodeUint64 writes the smallest integer representation of value.
func (w *Writer) EncodeUint64(value uint64) (err error) {
	_, err = w.write("encode uint64", AppendUint64(w.scratch[:0], value))
	return err
}

// EncodeFloat32 preserves value's float32 width and bits, including NaN payloads.
func (w *Writer) EncodeFloat32(value float32) (err error) {
	_, err = w.write("encode float32", AppendFloat32(w.scratch[:0], value))
	return err
}

// EncodeFloat64 preserves value's float64 width and bits, including NaN payloads.
func (w *Writer) EncodeFloat64(value float64) (err error) {
	_, err = w.write("encode float64", AppendFloat64(w.scratch[:0], value))
	return err
}

// EncodeArrayHeader writes only the header for count elements.
// Callers encode the children explicitly; this method does not validate them.
func (w *Writer) EncodeArrayHeader(count uint32) (err error) {
	_, err = w.write("encode array header", AppendArrayHeader(w.scratch[:0], count))
	return err
}

// EncodeMapHeader writes only the header for count key/value pairs.
// Callers encode both the keys and values explicitly.
func (w *Writer) EncodeMapHeader(count uint32) (err error) {
	_, err = w.write("encode map header", AppendMapHeader(w.scratch[:0], count))
	return err
}

// EncodeString preserves value's bytes without UTF-8 validation or normalization.
// It writes bounded chunks and retains no reference to value after returning.
func (w *Writer) EncodeString(value string) (err error) {
	if err = w.writePayloadHeader("encode string", uint64(len(value)), true); err != nil {
		return err
	}
	for len(value) > 0 {
		n := copy(w.payloadBuffer(), value)
		if _, err = w.write("encode string", w.state.buffer[:n]); err != nil {
			return err
		}
		value = value[n:]
	}
	return nil
}

// EncodeBytes writes value as binary in bounded chunks; nil means empty binary.
// Use EncodeNil for nil. No input storage is retained after returning.
func (w *Writer) EncodeBytes(value []byte) (err error) {
	if err = w.writePayloadHeader("encode bytes", uint64(len(value)), false); err != nil {
		return err
	}
	for len(value) > 0 {
		size := len(value)
		if size > payloadBufferSize {
			size = payloadBufferSize
		}
		if _, err = w.write("encode bytes", value[:size]); err != nil {
			return err
		}
		value = value[size:]
	}
	return nil
}

// EncodeStringFrom writes exactly length source bytes as a byte-preserving string.
// The returned n counts destination-accepted payload bytes, excluding the header.
// Zero length permits nil src; early EOF fails and extra source bytes remain unread.
// Failures after output remain terminal until Reset.
func (w *Writer) EncodeStringFrom(length uint64, src io.Reader) (n uint64, err error) {
	return w.encodeFrom("encode string from", length, src, true)
}

// EncodeBytesFrom writes exactly length source bytes as binary using bounded memory.
// The returned n counts destination-accepted payload bytes, excluding the header.
// Zero length permits nil src; early EOF fails and extra source bytes remain unread.
// Failures after output remain terminal until Reset.
func (w *Writer) EncodeBytesFrom(length uint64, src io.Reader) (n uint64, err error) {
	return w.encodeFrom("encode bytes from", length, src, false)
}

func (w *Writer) writePayloadHeader(operation string, length uint64, isString bool) (err error) {
	if err = w.ready(); err != nil {
		return err
	}
	var header []byte
	if isString {
		header, err = appendStringHeader(w.scratch[:0], length)
	} else {
		header, err = appendBytesHeader(w.scratch[:0], length)
	}
	if err != nil {
		return err
	}
	_, err = w.write(operation, header)
	return err
}

func (w *Writer) encodeFrom(operation string, length uint64, src io.Reader, isString bool) (n uint64, err error) {
	if err = w.ready(); err != nil {
		return 0, err
	}
	if length > 4294967295 {
		return 0, ErrOverflow
	}
	if length > 0 && src == nil {
		return 0, ErrInvalidArgument
	}
	if err = w.writePayloadHeader(operation, length, isString); err != nil {
		return 0, err
	}
	return w.copyPayloadFrom(operation, length, src)
}

func (w *Writer) copyPayloadFrom(operation string, remaining uint64, src io.Reader) (n uint64, err error) {
	var (
		read    int
		written int
		empty   int
		readErr error
		chunk   []byte
	)
	for remaining > 0 {
		chunk = w.payloadBuffer()
		if remaining < uint64(len(chunk)) {
			chunk = chunk[:int(remaining)]
		}
		read, readErr = src.Read(chunk)
		if read > 0 {
			written, err = w.write(operation, chunk[:read])
			n += uint64(written)
			if err != nil {
				return n, err
			}
			remaining -= uint64(read)
			empty = 0
		}
		if errors.Is(readErr, io.EOF) {
			if remaining == 0 {
				return n, nil
			}
			readErr = io.ErrUnexpectedEOF
		}
		if readErr != nil {
			return n, w.sourceError(operation, readErr)
		}
		if read == 0 {
			empty++
			if empty == 100 {
				return n, w.sourceError(operation, io.ErrNoProgress)
			}
		}
	}
	return n, nil
}

func (w *Writer) payloadBuffer() (buffer []byte) {
	if w.state.buffer == nil {
		w.state.buffer = make([]byte, payloadBufferSize)
	}
	return w.state.buffer
}

func (w *Writer) sourceError(operation string, err error) (out error) {
	w.state.err = fmt.Errorf("messagepack: %s source: %w", operation, err)
	return w.state.err
}

func (w *Writer) ready() (err error) {
	if w.state == nil {
		return ErrUninitialized
	}
	if w.state.err != nil {
		return w.state.err
	}
	if w.state.writer == nil {
		return ErrUninitialized
	}
	return nil
}

func (w *Writer) write(operation string, src []byte) (n int, err error) {
	if err = w.ready(); err != nil {
		return 0, err
	}
	n, err = w.state.writer.Write(src)
	if err == nil && n != len(src) {
		err = io.ErrShortWrite
	}
	if err != nil {
		w.state.err = fmt.Errorf("messagepack: %s destination: %w", operation, err)
		return n, w.state.err
	}
	return n, nil
}
