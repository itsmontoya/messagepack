package messagepack

import (
	"errors"
	"fmt"
	"io"
	"reflect"
)

// NewDecoder attaches a Decoder to r without read-ahead or taking ownership.
// A nil interface leaves the Decoder unattached.
func NewDecoder(r io.Reader) (out *Decoder) {
	var d Decoder
	d.reflected.reader.Reset(r)
	return &d
}

// Decoder reads Go values from a caller-owned reader, preferring Decodee hooks.
// Reflection decoding reuses instance-owned traversal state and cached type handlers.
// Its zero value is unattached. Decoding failures are terminal;
// instances must not be copied after first use or used concurrently.
type Decoder struct {
	reflected reflectedDecoder
	closed    bool
}

// Decode uses Decodee when available and otherwise reflects a non-nil pointer target.
// Invalid targets and unsupported root kinds return ErrUnsupportedType before input.
// Failures during decoding may consume partial input and prevent further decoding.
// Returned hook errors preserve schema context; ignored Reader failures still surface.
// Reflected byte slices reuse capacity; reads may overwrite retained aliases even on failure.
func (d *Decoder) Decode(value any) (err error) {
	if d.closed {
		return ErrClosed
	}

	if err = d.reflected.reader.ready(); err != nil {
		return err
	}

	var (
		decodee Decodee
		ok      bool
	)

	if decodee, ok = value.(Decodee); ok {
		err = decodee.DecodeMessagePack(d.reflected.reader)
	} else {
		var reflected = reflect.ValueOf(value)

		if !reflected.IsValid() || reflected.Kind() != reflect.Pointer || reflected.IsNil() {
			return ErrUnsupportedType
		}

		if !supportsReflection(reflected.Elem().Kind()) {
			return ErrUnsupportedType
		}

		d.reflected.remainingVisits = maximumReflectionVisits
		d.reflected.started = false
		err = d.reflected.decode(reflected.Elem(), 0)
		if err == nil {
			err = d.reflected.reader.state.err
		}

		if d.reflected.started && err == d.reflected.reader.state.err && errors.Is(err, io.EOF) {
			err = d.reflected.reader.fail(fmt.Errorf("messagepack: decode reflected value: %w", io.ErrUnexpectedEOF))
		}
	}

	if err == nil {
		return d.reflected.reader.state.err
	}

	if err == d.reflected.reader.state.err {
		return err
	}

	d.reflected.reader.state.err = fmt.Errorf("messagepack: decode value: %w", err)
	return d.reflected.reader.state.err
}

// Close releases the input and prevents further decoding without draining or closing it.
// Repeated calls return the stored decoding failure, if any.
func (d *Decoder) Close() (err error) {
	d.closed = true
	if d.reflected.reader.state == nil {
		return nil
	}

	d.reflected.reader.state.reader = nil
	return d.reflected.reader.state.err
}
