package messagepack

import (
	"errors"
	"fmt"
	"io"
)

// NewDecoder attaches a Decoder to r without read-ahead or taking ownership.
// A nil interface leaves it unattached; finite default limits apply.
func NewDecoder(r io.Reader) (d *Decoder) {
	d = &Decoder{}
	d.Reset(r)
	return d
}

// Decoder reads typed MessagePack values from a caller-owned reader.
// Its zero value is unattached. Failures after I/O are terminal until Reset;
// instances must not be copied after first use or used concurrently.
type Decoder struct {
	reader  io.Reader
	err     error
	eof     bool
	limits  DecoderLimits
	scratch [9]byte
}

// Reset attaches r and clears failure and EOF state while preserving limits.
// It never rewinds, drains, or closes a reader; callers supply a known boundary.
func (d *Decoder) Reset(r io.Reader) {
	d.reader = r
	d.err = nil
	d.eof = false
	if d.limits.MaxPayloadBytes == 0 {
		d.limits = DecoderLimits{
			MaxPayloadBytes: defaultMaxPayloadBytes,
			MaxSkipDepth:    defaultMaxSkipDepth,
			MaxSkipValues:   defaultMaxSkipValues,
			MaxSkipBytes:    defaultMaxSkipBytes,
		}
	}
}

// SetLimits atomically replaces limits without I/O or clearing a stored failure.
// Zero fields select defaults; invalid fields return ErrInvalidArgument unchanged.
func (d *Decoder) SetLimits(limits DecoderLimits) (err error) {
	var normalized DecoderLimits
	normalized, err = limits.normalized()
	if err != nil {
		return err
	}
	d.limits = normalized
	return nil
}

// DecodeNil accepts only a nil value, preserving its distinction from empty payloads.
func (d *Decoder) DecodeNil() (err error) {
	_, err = d.readTag("decode nil")
	if err != nil {
		return err
	}
	_, err = ReadNil(d.scratch[:1])
	return d.fail(err)
}

// DecodeBool accepts only a boolean; failures return false and may consume input.
func (d *Decoder) DecodeBool() (value bool, err error) {
	_, err = d.readTag("decode bool")
	if err != nil {
		return false, err
	}
	value, _, err = ReadBool(d.scratch[:1])
	return value, d.fail(err)
}

// DecodeInt64 accepts any integer encoding whose value fits int64.
// A failed operation returns zero and remains terminal until Reset.
func (d *Decoder) DecodeInt64() (value int64, err error) {
	var wire []byte
	wire, err = d.readInteger("decode int64")
	if err != nil {
		return 0, err
	}
	value, _, err = ReadInt64(wire)
	return value, d.fail(err)
}

// DecodeUint64 accepts any nonnegative integer encoding whose value fits uint64.
// A failed operation returns zero and remains terminal until Reset.
func (d *Decoder) DecodeUint64() (value uint64, err error) {
	var wire []byte
	wire, err = d.readInteger("decode uint64")
	if err != nil {
		return 0, err
	}
	value, _, err = ReadUint64(wire)
	return value, d.fail(err)
}

// DecodeFloat32 accepts float32 encodings and preserves their bits.
// It does not narrow float64; failures return positive zero.
func (d *Decoder) DecodeFloat32() (value float32, err error) {
	var tag byte
	tag, err = d.readTag("decode float32")
	if err != nil {
		return 0, err
	}
	if tag != 0xca {
		return 0, d.fail(readTypeError(tag))
	}
	if err = d.readScalarBody("decode float32", 5); err != nil {
		return 0, err
	}
	value, _, err = ReadFloat32(d.scratch[:5])
	return value, d.fail(err)
}

// DecodeFloat64 preserves float64 bits or widens float32 with Go's conversion.
// Widened NaN payload bits are unspecified; failures return positive zero.
func (d *Decoder) DecodeFloat64() (value float64, err error) {
	var (
		tag  byte
		size int
	)
	tag, err = d.readTag("decode float64")
	if err != nil {
		return 0, err
	}
	switch tag {
	case 0xca:
		size = 5
	case 0xcb:
		size = 9
	default:
		return 0, d.fail(readTypeError(tag))
	}
	if err = d.readScalarBody("decode float64", size); err != nil {
		return 0, err
	}
	value, _, err = ReadFloat64(d.scratch[:size])
	return value, d.fail(err)
}

func (d *Decoder) ready() (err error) {
	if d.err != nil {
		return d.err
	}
	if d.reader == nil {
		return ErrUninitialized
	}
	return nil
}

func (d *Decoder) fail(err error) (out error) {
	if err != nil {
		d.err = err
	}
	return err
}

func (d *Decoder) readTag(operation string) (tag byte, err error) {
	if err = d.ready(); err != nil {
		return 0, err
	}
	_, err = d.readFull(operation, d.scratch[:1], false)
	if err != nil {
		return 0, d.fail(err)
	}
	return d.scratch[0], nil
}

func (d *Decoder) readInteger(operation string) (wire []byte, err error) {
	var (
		tag  byte
		size = 1
	)
	tag, err = d.readTag(operation)
	if err != nil {
		return nil, err
	}
	switch {
	case tag <= 0x7f || tag >= 0xe0:
	case tag == 0xcc || tag == 0xd0:
		size = 2
	case tag == 0xcd || tag == 0xd1:
		size = 3
	case tag == 0xce || tag == 0xd2:
		size = 5
	case tag == 0xcf || tag == 0xd3:
		size = 9
	default:
		return nil, d.fail(readTypeError(tag))
	}
	if err = d.readScalarBody(operation, size); err != nil {
		return nil, err
	}
	return d.scratch[:size], nil
}

func (d *Decoder) readScalarBody(operation string, size int) (err error) {
	_, err = d.readFull(operation, d.scratch[1:size], true)
	return d.fail(err)
}

// readFull preserves partial counts for later bounded transfers; callers latch errors.
func (d *Decoder) readFull(operation string, dst []byte, started bool) (n int, err error) {
	var (
		read  int
		empty int
	)
	for n < len(dst) {
		if d.eof {
			err = io.EOF
			read = 0
		} else {
			read, err = d.reader.Read(dst[n:])
		}
		n += read
		if errors.Is(err, io.EOF) {
			d.eof = true
			if n == len(dst) {
				return n, nil
			}
			if started || n > 0 {
				err = io.ErrUnexpectedEOF
			}
			return n, fmt.Errorf("messagepack: %s source: %w", operation, err)
		}
		if err != nil {
			return n, fmt.Errorf("messagepack: %s source: %w", operation, err)
		}
		if read > 0 {
			empty = 0
			continue
		}
		empty++
		if empty == 100 {
			return n, fmt.Errorf("messagepack: %s source: %w", operation, io.ErrNoProgress)
		}
	}
	return n, nil
}
