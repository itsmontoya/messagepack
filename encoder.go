package messagepack

import (
	"fmt"
	"io"
	"reflect"
)

// NewEncoder attaches an Encoder to w without taking ownership or buffering output.
// A nil interface leaves the Encoder unattached.
func NewEncoder(w io.Writer) (out *Encoder) {
	var e Encoder
	e.reflected.writer.Reset(w)
	return &e
}

// Encoder writes Go values to a caller-owned writer, preferring Encodee hooks.
// Reflection encoding reuses instance-owned scratch and cached per-type handlers.
// Its zero value is unattached. Encoding failures are terminal;
// instances must not be copied after first use or used concurrently.
type Encoder struct {
	reflected reflectedEncoder
	closed    bool
}

// Encode uses Encodee when available and otherwise reflects supported Go values.
// Unsupported root kinds return ErrUnsupportedType without output or a terminal failure.
// Failures during encoding may leave partial output and prevent further encoding.
func (e *Encoder) Encode(value any) (err error) {
	if e.closed {
		return ErrClosed
	}

	if err = e.reflected.writer.ready(); err != nil {
		return err
	}

	var (
		encodee Encodee
		ok      bool
	)

	if encodee, ok = value.(Encodee); ok {
		err = encodee.EncodeMessagePack(e.reflected.writer)
	} else {
		var reflected = reflect.ValueOf(value)

		if reflected.IsValid() && !supportsReflection(reflected.Kind()) {
			return ErrUnsupportedType
		}

		err = e.reflected.encode(reflected, 0)
	}

	if e.reflected.writer.state.err != nil {
		return e.reflected.writer.state.err
	}

	if err != nil {
		e.reflected.writer.state.err = fmt.Errorf("messagepack: encode value: %w", err)
		return e.reflected.writer.state.err
	}

	return nil
}

// Close releases the output and prevents further encoding without flushing or closing it.
// Repeated calls return the stored encoding failure, if any.
func (e *Encoder) Close() (err error) {
	e.closed = true
	if e.reflected.writer.state == nil {
		return nil
	}

	e.reflected.writer.state.writer = nil
	return e.reflected.writer.state.err
}
