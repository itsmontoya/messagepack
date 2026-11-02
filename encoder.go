package messagepack

import (
	"fmt"
	"io"
)

// NewEncoder attaches an Encoder to w without taking ownership or buffering output.
// A nil interface leaves the Encoder unattached until Reset supplies a writer.
func NewEncoder(w io.Writer) (e *Encoder) {
	return &Encoder{writer: w}
}

// Encoder writes typed MessagePack values to a caller-owned writer.
// Its zero value is unattached. Write failures remain terminal until Reset;
// instances must not be copied after first use or used concurrently.
type Encoder struct {
	writer  io.Writer
	err     error
	scratch [9]byte
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
