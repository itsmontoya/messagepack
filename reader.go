package messagepack

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// NewReader attaches a Reader to src without read-ahead or taking ownership.
// A nil interface leaves it unattached; finite default limits apply.
func NewReader(src io.Reader) (r *Reader) {
	r = &Reader{}
	r.Reset(src)
	return r
}

// Reader reads typed MessagePack values from a caller-owned reader.
// Its zero value is unattached. Initialized copies share input, limits, and failure state.
// Failures after I/O are terminal until Reset; shared instances require synchronization.
type Reader struct {
	state   *readerState
	scratch [9]byte
}

// Reset attaches src and clears shared failure and EOF state while preserving limits.
// It never rewinds, drains, or closes a reader; callers supply a known boundary.
func (r *Reader) Reset(src io.Reader) {
	if r.state == nil {
		r.state = &readerState{}
	}
	r.state.reader = src
	r.state.err = nil
	r.state.eof = false
	if r.state.limits.MaxPayloadBytes == 0 {
		r.state.limits = ReaderLimits{
			MaxPayloadBytes: defaultMaxPayloadBytes,
			MaxSkipDepth:    defaultMaxSkipDepth,
			MaxSkipValues:   defaultMaxSkipValues,
			MaxSkipBytes:    defaultMaxSkipBytes,
		}
	}
}

// SetLimits atomically replaces limits without I/O or clearing a stored failure.
// Zero fields select defaults; invalid fields return ErrInvalidArgument unchanged.
func (r *Reader) SetLimits(limits ReaderLimits) (err error) {
	var normalized ReaderLimits
	normalized, err = limits.normalized()
	if err != nil {
		return err
	}
	if r.state == nil {
		r.state = &readerState{}
	}
	r.state.limits = normalized
	return nil
}

// DecodeNil accepts only a nil value, preserving its distinction from empty payloads.
func (r *Reader) DecodeNil() (err error) {
	_, err = r.readTag("decode nil")
	if err != nil {
		return err
	}
	_, err = ReadNil(r.scratch[:1])
	return r.fail(err)
}

// DecodeBool accepts only a boolean; failures return false and may consume input.
func (r *Reader) DecodeBool() (value bool, err error) {
	_, err = r.readTag("decode bool")
	if err != nil {
		return false, err
	}
	value, _, err = ReadBool(r.scratch[:1])
	return value, r.fail(err)
}

// DecodeInt64 accepts any integer encoding whose value fits int64.
// A failed operation returns zero and remains terminal until Reset.
func (r *Reader) DecodeInt64() (value int64, err error) {
	var wire []byte
	wire, err = r.readInteger("decode int64")
	if err != nil {
		return 0, err
	}
	value, _, err = ReadInt64(wire)
	return value, r.fail(err)
}

// DecodeUint64 accepts any nonnegative integer encoding whose value fits uint64.
// A failed operation returns zero and remains terminal until Reset.
func (r *Reader) DecodeUint64() (value uint64, err error) {
	var wire []byte
	wire, err = r.readInteger("decode uint64")
	if err != nil {
		return 0, err
	}
	value, _, err = ReadUint64(wire)
	return value, r.fail(err)
}

// DecodeFloat32 accepts float32 encodings and preserves their bits.
// It does not narrow float64; failures return positive zero.
func (r *Reader) DecodeFloat32() (value float32, err error) {
	var tag byte
	tag, err = r.readTag("decode float32")
	if err != nil {
		return 0, err
	}
	if tag != 0xca {
		return 0, r.fail(readTypeError(tag))
	}
	if err = r.readScalarBody("decode float32", 5); err != nil {
		return 0, err
	}
	value, _, err = ReadFloat32(r.scratch[:5])
	return value, r.fail(err)
}

// DecodeFloat64 preserves float64 bits or widens float32 with Go's conversion.
// Widened NaN payload bits are unspecified; failures return positive zero.
func (r *Reader) DecodeFloat64() (value float64, err error) {
	var (
		tag  byte
		size int
	)
	tag, err = r.readTag("decode float64")
	if err != nil {
		return 0, err
	}
	switch tag {
	case 0xca:
		size = 5
	case 0xcb:
		size = 9
	default:
		return 0, r.fail(readTypeError(tag))
	}
	if err = r.readScalarBody("decode float64", size); err != nil {
		return 0, err
	}
	value, _, err = ReadFloat64(r.scratch[:size])
	return value, r.fail(err)
}

// DecodeArrayHeader consumes only a header and returns its element count.
// Success does not require or validate children; callers traverse them explicitly.
func (r *Reader) DecodeArrayHeader() (count uint32, err error) {
	return r.readContainerHeader("decode array header", false)
}

// DecodeMapHeader consumes only a header and returns its key/value pair count.
// It does not validate children or apply Skip limits. Failures return zero.
func (r *Reader) DecodeMapHeader() (count uint32, err error) {
	return r.readContainerHeader("decode map header", true)
}

func (r *Reader) readContainerHeader(operation string, isMap bool) (count uint32, err error) {
	var (
		tag  byte
		size = 1
	)
	tag, err = r.readTag(operation)
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
		return 0, r.fail(readTypeError(tag))
	}
	if err = r.readScalarBody(operation, size); err != nil {
		return 0, err
	}
	if isMap {
		count, _, err = ReadMapHeader(r.scratch[:size])
	} else {
		count, _, err = ReadArrayHeader(r.scratch[:size])
	}
	return count, r.fail(err)
}

// DecodeString returns owned, byte-preserving contents within MaxPayloadBytes.
// Failures return an empty string and may consume input; no UTF-8 validation occurs.
func (r *Reader) DecodeString() (value string, err error) {
	var payload []byte
	payload, err = r.readOwnedPayload("decode string", true)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// DecodeBytes returns owned binary contents within MaxPayloadBytes.
// Empty binary succeeds with a non-nil slice; failures return nil.
func (r *Reader) DecodeBytes() (value []byte, err error) {
	return r.readOwnedPayload("decode bytes", false)
}

// DecodeStringTo copies a string's original bytes to dst using bounded memory.
// The returned n counts accepted payload bytes, including partial failed writes.
// A nil destination fails before I/O; transfer failures remain terminal until Reset.
func (r *Reader) DecodeStringTo(dst io.Writer) (n uint64, err error) {
	return r.decodeTo("decode string to", dst, true)
}

// DecodeBytesTo copies binary to dst independently of MaxPayloadBytes.
// The returned n excludes the header; a failed transfer does not drain its input.
// A nil destination is invalid even when the wire payload is empty.
func (r *Reader) DecodeBytesTo(dst io.Writer) (n uint64, err error) {
	return r.decodeTo("decode bytes to", dst, false)
}

// Skip consumes one complete supported value without materializing its contents.
// It applies per-call depth, value, and wire-byte limits and leaves the next value.
// A failure is terminal until Reset; extensions remain unsupported.
func (r *Reader) Skip() (err error) {
	if err = r.ready(); err != nil {
		return err
	}
	var (
		state = skipState{
			remainingBytes:  r.state.limits.MaxSkipBytes,
			remainingValues: r.state.limits.MaxSkipValues,
			outstanding:     1,
		}
		children  uint64
		container bool
	)
	for state.outstanding > 0 {
		for state.depth > 0 && state.children[state.depth-1] == 0 {
			state.depth--
		}
		state.outstanding--
		state.remainingValues--
		if state.depth > 0 {
			state.children[state.depth-1]--
		}
		children, container, err = r.skipValue(&state)
		if err != nil {
			return r.fail(err)
		}
		if err = r.skipChildren(&state, children, container); err != nil {
			return r.fail(err)
		}
	}
	return nil
}

func (r *Reader) skipValue(state *skipState) (children uint64, container bool, err error) {
	if err = r.readSkip(state, r.scratch[:1]); err != nil {
		return 0, false, err
	}
	tag := r.scratch[0]
	switch {
	case tag <= 0x7f || tag >= 0xe0 || tag == 0xc0 || tag == 0xc2 || tag == 0xc3:
		return 0, false, nil
	case tag >= 0x80 && tag <= 0x8f:
		return uint64(tag&0x0f) * 2, true, nil
	case tag >= 0x90 && tag <= 0x9f:
		return uint64(tag & 0x0f), true, nil
	case tag >= 0xa0 && tag <= 0xbf:
		return 0, false, r.skipPayload(state, uint64(tag&0x1f))
	default:
		return r.skipTaggedValue(state, tag)
	}
}

func (r *Reader) skipTaggedValue(state *skipState, tag byte) (children uint64, container bool, err error) {
	switch tag {
	case 0xcc, 0xd0:
		return 0, false, r.readSkip(state, r.scratch[1:2])
	case 0xcd, 0xd1:
		return 0, false, r.readSkip(state, r.scratch[1:3])
	case 0xce, 0xd2, 0xca:
		return 0, false, r.readSkip(state, r.scratch[1:5])
	case 0xcf, 0xd3, 0xcb:
		return 0, false, r.readSkip(state, r.scratch[1:9])
	case 0xc4, 0xd9:
		return 0, false, r.skipPayloadValue(state, 1)
	case 0xc5, 0xda:
		return 0, false, r.skipPayloadValue(state, 2)
	case 0xc6, 0xdb:
		return 0, false, r.skipPayloadValue(state, 4)
	case 0xdc, 0xde:
		return r.skipContainerValue(state, 2, tag == 0xde)
	case 0xdd, 0xdf:
		return r.skipContainerValue(state, 4, tag == 0xdf)
	default:
		return 0, false, readTypeError(tag)
	}
}

func (r *Reader) skipPayloadValue(state *skipState, width int) (err error) {
	var length uint64
	length, err = r.readSkipLength(state, width)
	if err != nil {
		return err
	}
	return r.skipPayload(state, length)
}

func (r *Reader) skipContainerValue(state *skipState, width int, isMap bool) (children uint64, container bool, err error) {
	children, err = r.readSkipLength(state, width)
	if err != nil {
		return 0, false, err
	}
	if isMap {
		children *= 2
	}
	return children, true, nil
}

func (r *Reader) readSkipLength(state *skipState, width int) (length uint64, err error) {
	if err = r.readSkip(state, r.scratch[1:1+width]); err != nil {
		return 0, err
	}
	switch width {
	case 1:
		return uint64(r.scratch[1]), nil
	case 2:
		return uint64(binary.BigEndian.Uint16(r.scratch[1:3])), nil
	default:
		return uint64(binary.BigEndian.Uint32(r.scratch[1:5])), nil
	}
}

func (r *Reader) skipChildren(state *skipState, children uint64, container bool) (err error) {
	if container && state.depth == r.state.limits.MaxSkipDepth {
		return ErrLimitExceeded
	}
	if state.outstanding > state.remainingValues || children > state.remainingValues-state.outstanding {
		return ErrLimitExceeded
	}
	if state.outstanding > state.remainingBytes || children > state.remainingBytes-state.outstanding {
		return ErrLimitExceeded
	}
	state.outstanding += children
	if children > 0 {
		state.children[state.depth] = children
		state.depth++
	}
	return nil
}

func (r *Reader) skipPayload(state *skipState, length uint64) (err error) {
	if length > state.remainingBytes {
		return ErrLimitExceeded
	}
	for length > 0 {
		chunk := r.payloadBuffer()
		if length < uint64(len(chunk)) {
			chunk = chunk[:int(length)]
		}
		if err = r.readSkip(state, chunk); err != nil {
			return err
		}
		length -= uint64(len(chunk))
	}
	return nil
}

func (r *Reader) readSkip(state *skipState, dst []byte) (err error) {
	if uint64(len(dst)) > state.remainingBytes {
		return ErrLimitExceeded
	}
	var n int
	n, err = r.readFull("skip", dst, state.started)
	state.remainingBytes -= uint64(n)
	if n > 0 {
		state.started = true
	}
	return err
}

func (r *Reader) readPayloadHeader(operation string, isString bool) (length uint64, err error) {
	var (
		tag  byte
		size int
	)
	tag, err = r.readTag(operation)
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
		return 0, r.fail(readTypeError(tag))
	}
	if err = r.readScalarBody(operation, size); err != nil {
		return 0, err
	}
	switch size {
	case 2:
		return uint64(r.scratch[1]), nil
	case 3:
		return uint64(binary.BigEndian.Uint16(r.scratch[1:3])), nil
	default:
		return uint64(binary.BigEndian.Uint32(r.scratch[1:5])), nil
	}
}

func (r *Reader) readOwnedPayload(operation string, isString bool) (value []byte, err error) {
	var length uint64
	length, err = r.readPayloadHeader(operation, isString)
	if err != nil {
		return nil, err
	}
	if length > r.state.limits.MaxPayloadBytes {
		return nil, r.fail(ErrLimitExceeded)
	}
	if length > uint64(^uint(0)>>1) {
		return nil, r.fail(ErrOverflow)
	}
	value = make([]byte, int(length))
	for offset := 0; offset < len(value); {
		size := len(value) - offset
		if size > payloadBufferSize {
			size = payloadBufferSize
		}
		if _, err = r.readFull(operation, value[offset:offset+size], true); err != nil {
			return nil, r.fail(err)
		}
		offset += size
	}
	return value, nil
}

func (r *Reader) decodeTo(operation string, dst io.Writer, isString bool) (n uint64, err error) {
	if err = r.ready(); err != nil {
		return 0, err
	}
	if dst == nil {
		return 0, ErrInvalidArgument
	}
	var length uint64
	length, err = r.readPayloadHeader(operation, isString)
	if err != nil {
		return 0, err
	}
	return r.copyPayloadTo(operation, dst, length)
}

func (r *Reader) copyPayloadTo(operation string, dst io.Writer, remaining uint64) (n uint64, err error) {
	var (
		chunk   []byte
		read    int
		written int
		readErr error
	)
	for remaining > 0 {
		chunk = r.payloadBuffer()
		if remaining < uint64(len(chunk)) {
			chunk = chunk[:int(remaining)]
		}
		read, readErr = r.readFull(operation, chunk, true)
		if read > 0 {
			written, err = dst.Write(chunk[:read])
			n += uint64(written)
			if err == nil && written != read {
				err = io.ErrShortWrite
			}
			if err != nil {
				return n, r.fail(fmt.Errorf("messagepack: %s destination: %w", operation, err))
			}
			remaining -= uint64(read)
		}
		if readErr != nil {
			return n, r.fail(readErr)
		}
	}
	return n, nil
}

func (r *Reader) payloadBuffer() (buffer []byte) {
	if r.state.buffer == nil {
		r.state.buffer = make([]byte, payloadBufferSize)
	}
	return r.state.buffer
}

func (r *Reader) ready() (err error) {
	if r.state == nil {
		return ErrUninitialized
	}
	if r.state.err != nil {
		return r.state.err
	}
	if r.state.reader == nil {
		return ErrUninitialized
	}
	return nil
}

func (r *Reader) fail(err error) (out error) {
	if err != nil {
		r.state.err = err
	}
	return err
}

func (r *Reader) readTag(operation string) (tag byte, err error) {
	if err = r.ready(); err != nil {
		return 0, err
	}
	_, err = r.readFull(operation, r.scratch[:1], false)
	if err != nil {
		return 0, r.fail(err)
	}
	return r.scratch[0], nil
}

func (r *Reader) readInteger(operation string) (wire []byte, err error) {
	var (
		tag  byte
		size = 1
	)
	tag, err = r.readTag(operation)
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
		return nil, r.fail(readTypeError(tag))
	}
	if err = r.readScalarBody(operation, size); err != nil {
		return nil, err
	}
	return r.scratch[:size], nil
}

func (r *Reader) readScalarBody(operation string, size int) (err error) {
	_, err = r.readFull(operation, r.scratch[1:size], true)
	return r.fail(err)
}

// readFull preserves partial counts for later bounded transfers; callers latch errors.
func (r *Reader) readFull(operation string, dst []byte, started bool) (n int, err error) {
	var (
		read  int
		empty int
	)
	for n < len(dst) {
		if r.state.eof {
			err = io.EOF
			read = 0
		} else {
			read, err = r.state.reader.Read(dst[n:])
		}
		n += read
		if errors.Is(err, io.EOF) {
			r.state.eof = true
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
