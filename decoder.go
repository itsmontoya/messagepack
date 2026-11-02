package messagepack

import (
	"encoding/binary"
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
	buffer  []byte
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

// DecodeArrayHeader consumes only a header and returns its element count.
// Success does not require or validate children; callers traverse them explicitly.
func (d *Decoder) DecodeArrayHeader() (count uint32, err error) {
	return d.readContainerHeader("decode array header", false)
}

// DecodeMapHeader consumes only a header and returns its key/value pair count.
// It does not validate children or apply Skip limits. Failures return zero.
func (d *Decoder) DecodeMapHeader() (count uint32, err error) {
	return d.readContainerHeader("decode map header", true)
}

func (d *Decoder) readContainerHeader(operation string, isMap bool) (count uint32, err error) {
	var (
		tag  byte
		size = 1
	)
	tag, err = d.readTag(operation)
	if err != nil {
		return 0, err
	}
	switch {
	case !isMap && tag >= 0x90 && tag <= 0x9f || isMap && tag >= 0x80 && tag <= 0x8f:
	case !isMap && tag == 0xdc || isMap && tag == 0xde:
		size = 3
	case !isMap && tag == 0xdd || isMap && tag == 0xdf:
		size = 5
	default:
		return 0, d.fail(readTypeError(tag))
	}
	if err = d.readScalarBody(operation, size); err != nil {
		return 0, err
	}
	if isMap {
		count, _, err = ReadMapHeader(d.scratch[:size])
	} else {
		count, _, err = ReadArrayHeader(d.scratch[:size])
	}
	return count, d.fail(err)
}

// DecodeString returns owned, byte-preserving contents within MaxPayloadBytes.
// Failures return an empty string and may consume input; no UTF-8 validation occurs.
func (d *Decoder) DecodeString() (value string, err error) {
	var payload []byte
	payload, err = d.readOwnedPayload("decode string", true)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// DecodeBytes returns owned binary contents within MaxPayloadBytes.
// Empty binary succeeds with a non-nil slice; failures return nil.
func (d *Decoder) DecodeBytes() (value []byte, err error) {
	return d.readOwnedPayload("decode bytes", false)
}

// DecodeStringTo copies a string's original bytes to dst using bounded memory.
// The returned n counts accepted payload bytes, including partial failed writes.
// A nil destination fails before I/O; transfer failures remain terminal until Reset.
func (d *Decoder) DecodeStringTo(dst io.Writer) (n uint64, err error) {
	return d.decodeTo("decode string to", dst, true)
}

// DecodeBytesTo copies binary to dst independently of MaxPayloadBytes.
// The returned n excludes the header; a failed transfer does not drain its input.
// A nil destination is invalid even when the wire payload is empty.
func (d *Decoder) DecodeBytesTo(dst io.Writer) (n uint64, err error) {
	return d.decodeTo("decode bytes to", dst, false)
}

func (d *Decoder) readPayloadHeader(operation string, isString bool) (length uint64, err error) {
	var (
		tag  byte
		size int
	)
	tag, err = d.readTag(operation)
	if err != nil {
		return 0, err
	}
	switch {
	case isString && tag >= 0xa0 && tag <= 0xbf:
		return uint64(tag & 0x1f), nil
	case isString && tag == 0xd9 || !isString && tag == 0xc4:
		size = 2
	case isString && tag == 0xda || !isString && tag == 0xc5:
		size = 3
	case isString && tag == 0xdb || !isString && tag == 0xc6:
		size = 5
	default:
		return 0, d.fail(readTypeError(tag))
	}
	if err = d.readScalarBody(operation, size); err != nil {
		return 0, err
	}
	switch size {
	case 2:
		return uint64(d.scratch[1]), nil
	case 3:
		return uint64(binary.BigEndian.Uint16(d.scratch[1:3])), nil
	default:
		return uint64(binary.BigEndian.Uint32(d.scratch[1:5])), nil
	}
}

func (d *Decoder) readOwnedPayload(operation string, isString bool) (value []byte, err error) {
	var length uint64
	length, err = d.readPayloadHeader(operation, isString)
	if err != nil {
		return nil, err
	}
	if length > d.limits.MaxPayloadBytes {
		return nil, d.fail(ErrLimitExceeded)
	}
	if length > uint64(^uint(0)>>1) {
		return nil, d.fail(ErrOverflow)
	}
	value = make([]byte, int(length))
	for offset := 0; offset < len(value); {
		size := len(value) - offset
		if size > payloadBufferSize {
			size = payloadBufferSize
		}
		if _, err = d.readFull(operation, value[offset:offset+size], true); err != nil {
			return nil, d.fail(err)
		}
		offset += size
	}
	return value, nil
}

func (d *Decoder) decodeTo(operation string, dst io.Writer, isString bool) (n uint64, err error) {
	if err = d.ready(); err != nil {
		return 0, err
	}
	if dst == nil {
		return 0, ErrInvalidArgument
	}
	var length uint64
	length, err = d.readPayloadHeader(operation, isString)
	if err != nil {
		return 0, err
	}
	return d.copyPayloadTo(operation, dst, length)
}

func (d *Decoder) copyPayloadTo(operation string, dst io.Writer, remaining uint64) (n uint64, err error) {
	var (
		chunk   []byte
		read    int
		written int
		readErr error
	)
	for remaining > 0 {
		chunk = d.payloadBuffer()
		if remaining < uint64(len(chunk)) {
			chunk = chunk[:int(remaining)]
		}
		read, readErr = d.readFull(operation, chunk, true)
		if read > 0 {
			written, err = dst.Write(chunk[:read])
			n += uint64(written)
			if err == nil && written != read {
				err = io.ErrShortWrite
			}
			if err != nil {
				return n, d.fail(fmt.Errorf("messagepack: %s destination: %w", operation, err))
			}
			remaining -= uint64(read)
		}
		if readErr != nil {
			return n, d.fail(readErr)
		}
	}
	return n, nil
}

func (d *Decoder) payloadBuffer() (buffer []byte) {
	if d.buffer == nil {
		d.buffer = make([]byte, payloadBufferSize)
	}
	return d.buffer
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
