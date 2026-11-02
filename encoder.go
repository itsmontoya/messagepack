package messagepack

import (
	"errors"
	"fmt"
	"io"
)

const payloadBufferSize = 32768

// NewEncoder attaches an Encoder to w without taking ownership or buffering output.
// A nil interface leaves the Encoder unattached until Reset supplies a writer.
func NewEncoder(w io.Writer) (e *Encoder) {
	return &Encoder{writer: w}
}

// Encoder writes typed MessagePack values to a caller-owned writer.
// Its zero value is unattached. I/O failures remain terminal until Reset;
// instances must not be copied after first use or used concurrently.
type Encoder struct {
	writer  io.Writer
	err     error
	scratch [9]byte
	buffer  []byte
}

// Reset attaches w and clears the stored failure without flushing or closing.
// It cannot repair a partial message; callers must supply a known value boundary.
func (e *Encoder) Reset(w io.Writer) {
	e.writer = w
	e.err = nil
}

// EncodeNil writes a nil value, preserving the distinction from empty payloads.
func (e *Encoder) EncodeNil() (err error) {
	_, err = e.write("encode nil", AppendNil(e.scratch[:0]))
	return err
}

// EncodeBool writes value without buffering output for later operations.
func (e *Encoder) EncodeBool(value bool) (err error) {
	_, err = e.write("encode bool", AppendBool(e.scratch[:0], value))
	return err
}

// EncodeInt64 writes the smallest integer representation of value.
// Nonnegative values encode identically to EncodeUint64.
func (e *Encoder) EncodeInt64(value int64) (err error) {
	_, err = e.write("encode int64", AppendInt64(e.scratch[:0], value))
	return err
}

// EncodeUint64 writes the smallest integer representation of value.
func (e *Encoder) EncodeUint64(value uint64) (err error) {
	_, err = e.write("encode uint64", AppendUint64(e.scratch[:0], value))
	return err
}

// EncodeFloat32 preserves value's float32 width and bits, including NaN payloads.
func (e *Encoder) EncodeFloat32(value float32) (err error) {
	_, err = e.write("encode float32", AppendFloat32(e.scratch[:0], value))
	return err
}

// EncodeFloat64 preserves value's float64 width and bits, including NaN payloads.
func (e *Encoder) EncodeFloat64(value float64) (err error) {
	_, err = e.write("encode float64", AppendFloat64(e.scratch[:0], value))
	return err
}

// EncodeArrayHeader writes only the header for count elements.
// Callers encode the children explicitly; this method does not validate them.
func (e *Encoder) EncodeArrayHeader(count uint32) (err error) {
	_, err = e.write("encode array header", AppendArrayHeader(e.scratch[:0], count))
	return err
}

// EncodeMapHeader writes only the header for count key/value pairs.
// Callers encode both the keys and values explicitly.
func (e *Encoder) EncodeMapHeader(count uint32) (err error) {
	_, err = e.write("encode map header", AppendMapHeader(e.scratch[:0], count))
	return err
}

// EncodeString preserves value's bytes without UTF-8 validation or normalization.
// It writes bounded chunks and retains no reference to value after returning.
func (e *Encoder) EncodeString(value string) (err error) {
	if err = e.writePayloadHeader("encode string", uint64(len(value)), true); err != nil {
		return err
	}
	for len(value) > 0 {
		n := copy(e.payloadBuffer(), value)
		if _, err = e.write("encode string", e.buffer[:n]); err != nil {
			return err
		}
		value = value[n:]
	}
	return nil
}

// EncodeBytes writes value as binary in bounded chunks; nil means empty binary.
// Use EncodeNil for nil. No input storage is retained after returning.
func (e *Encoder) EncodeBytes(value []byte) (err error) {
	if err = e.writePayloadHeader("encode bytes", uint64(len(value)), false); err != nil {
		return err
	}
	for len(value) > 0 {
		size := len(value)
		if size > payloadBufferSize {
			size = payloadBufferSize
		}
		if _, err = e.write("encode bytes", value[:size]); err != nil {
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
func (e *Encoder) EncodeStringFrom(length uint64, src io.Reader) (n uint64, err error) {
	return e.encodeFrom("encode string from", length, src, true)
}

// EncodeBytesFrom writes exactly length source bytes as binary using bounded memory.
// The returned n counts destination-accepted payload bytes, excluding the header.
// Zero length permits nil src; early EOF fails and extra source bytes remain unread.
// Failures after output remain terminal until Reset.
func (e *Encoder) EncodeBytesFrom(length uint64, src io.Reader) (n uint64, err error) {
	return e.encodeFrom("encode bytes from", length, src, false)
}

func (e *Encoder) writePayloadHeader(operation string, length uint64, isString bool) (err error) {
	if err = e.ready(); err != nil {
		return err
	}
	var header []byte
	if isString {
		header, err = appendStringHeader(e.scratch[:0], length)
	} else {
		header, err = appendBytesHeader(e.scratch[:0], length)
	}
	if err != nil {
		return err
	}
	_, err = e.write(operation, header)
	return err
}

func (e *Encoder) encodeFrom(operation string, length uint64, src io.Reader, isString bool) (n uint64, err error) {
	if err = e.ready(); err != nil {
		return 0, err
	}
	if length > 4294967295 {
		return 0, ErrOverflow
	}
	if length > 0 && src == nil {
		return 0, ErrInvalidArgument
	}
	if err = e.writePayloadHeader(operation, length, isString); err != nil {
		return 0, err
	}
	return e.copyPayloadFrom(operation, length, src)
}

func (e *Encoder) copyPayloadFrom(operation string, remaining uint64, src io.Reader) (n uint64, err error) {
	var (
		read    int
		written int
		empty   int
		readErr error
		chunk   []byte
	)
	for remaining > 0 {
		chunk = e.payloadBuffer()
		if remaining < uint64(len(chunk)) {
			chunk = chunk[:int(remaining)]
		}
		read, readErr = src.Read(chunk)
		if read > 0 {
			written, err = e.write(operation, chunk[:read])
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
			return n, e.sourceError(operation, readErr)
		}
		if read == 0 {
			empty++
			if empty == 100 {
				return n, e.sourceError(operation, io.ErrNoProgress)
			}
		}
	}
	return n, nil
}

func (e *Encoder) payloadBuffer() (buffer []byte) {
	if e.buffer == nil {
		e.buffer = make([]byte, payloadBufferSize)
	}
	return e.buffer
}

func (e *Encoder) sourceError(operation string, err error) (out error) {
	e.err = fmt.Errorf("messagepack: %s source: %w", operation, err)
	return e.err
}

func (e *Encoder) ready() (err error) {
	if e.err != nil {
		return e.err
	}
	if e.writer == nil {
		return ErrUninitialized
	}
	return nil
}

func (e *Encoder) write(operation string, src []byte) (n int, err error) {
	if err = e.ready(); err != nil {
		return 0, err
	}
	n, err = e.writer.Write(src)
	if err == nil && n != len(src) {
		err = io.ErrShortWrite
	}
	if err != nil {
		e.err = fmt.Errorf("messagepack: %s destination: %w", operation, err)
		return n, e.err
	}
	return n, nil
}
