package messagepack

import (
	"fmt"
	"io"
)

// NewDecoder attaches a Decoder to r without read-ahead or taking ownership.
// A nil interface leaves the Decoder unattached.
func NewDecoder(r io.Reader) (out *Decoder) {
	var d Decoder
	d.reader.Reset(r)
	return &d
}

// Decoder reads values implementing Decodee from a caller-owned reader.
// Its zero value is unattached. Decoding failures are terminal;
// instances must not be copied after first use or used concurrently.
type Decoder struct {
	reader Reader
	closed bool
}

// Decode delegates to value's DecodeMessagePack method, sharing input and failures.
// Other values return ErrUnsupportedType without input or a terminal failure.
// A Decodee failure may consume partial input and prevents further decoding.
// Returned hook errors preserve schema context; ignored Reader failures still surface.
func (d *Decoder) Decode(value any) (err error) {
	if d.closed {
		return ErrClosed
	}

	if err = d.reader.ready(); err != nil {
		return err
	}

	var (
		decodee Decodee
		ok      bool
	)

	if decodee, ok = value.(Decodee); !ok {
		return ErrUnsupportedType
	}

	err = decodee.DecodeMessagePack(d.reader)
	if err == nil {
		return d.reader.state.err
	}

	if err == d.reader.state.err {
		return err
	}

	d.reader.state.err = fmt.Errorf("messagepack: decode value: %w", err)
	return d.reader.state.err
}

// Close releases the input and prevents further decoding without draining or closing it.
// Repeated calls return the stored decoding failure, if any.
func (d *Decoder) Close() (err error) {
	d.closed = true
	if d.reader.state == nil {
		return nil
	}

	d.reader.state.reader = nil
	return d.reader.state.err
}
