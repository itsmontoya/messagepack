package messagepack

import (
	"fmt"
	"io"
)

// NewEncoder attaches an Encoder to w without taking ownership or buffering output.
// A nil interface leaves the Encoder unattached.
func NewEncoder(w io.Writer) (out *Encoder) {
	var e Encoder
	e.writer.Reset(w)
	return &e
}

// Encoder writes values implementing Encodee to a caller-owned writer.
// Its zero value is unattached. Encoding failures are terminal;
// instances must not be copied after first use or used concurrently.
type Encoder struct {
	writer Writer
	closed bool
}

// Encode delegates to value's EncodeMessagePack method, sharing output and failures.
// Other values return ErrUnsupportedType without output or a terminal failure.
// An Encodee failure may leave partial output and prevents further encoding.
func (e *Encoder) Encode(value any) (err error) {
	if e.closed {
		return ErrClosed
	}

	if err = e.writer.ready(); err != nil {
		return err
	}

	var (
		encodee Encodee
		ok      bool
	)

	if encodee, ok = value.(Encodee); !ok {
		return ErrUnsupportedType
	}

	err = encodee.EncodeMessagePack(e.writer)
	if e.writer.state.err != nil {
		return e.writer.state.err
	}

	if err != nil {
		e.writer.state.err = fmt.Errorf("messagepack: encode value: %w", err)
		return e.writer.state.err
	}

	return nil
}

// Close releases the output and prevents further encoding without flushing or closing it.
// Repeated calls return the stored encoding failure, if any.
func (e *Encoder) Close() (err error) {
	e.closed = true
	if e.writer.state == nil {
		return nil
	}

	e.writer.state.writer = nil
	return e.writer.state.err
}
