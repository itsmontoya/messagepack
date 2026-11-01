package messagepack

import "encoding/binary"

// AppendString appends value's bytes without UTF-8 validation or normalization.
// Payload and dst storage must be disjoint. It preserves dst's prefix and reuses
// capacity; an oversized payload returns dst unchanged with ErrOverflow.
func AppendString(dst []byte, value string) (out []byte, err error) {
	out, err = appendStringHeader(dst, uint64(len(value)))
	if err != nil {
		return dst, err
	}
	return append(out, value...), nil
}

func appendStringHeader(dst []byte, length uint64) (out []byte, err error) {
	switch {
	case length <= 31:
		return append(dst, 0xa0|byte(length)), nil
	case length <= 255:
		return append(dst, 0xd9, byte(length)), nil
	case length <= 65535:
		return binary.BigEndian.AppendUint16(append(dst, 0xda), uint16(length)), nil
	case length <= 4294967295:
		return binary.BigEndian.AppendUint32(append(dst, 0xdb), uint32(length)), nil
	default:
		return dst, ErrOverflow
	}
}

// ReadString consumes a string and returns an owned copy, leaving src unchanged.
// It preserves bytes without UTF-8 validation or normalization. Failures return
// an empty string and consume no bytes.
func ReadString(src []byte) (value string, n int, err error) {
	var view []byte
	view, n, err = ReadStringBytes(src)
	if err != nil {
		return "", 0, err
	}
	return string(view), n, nil
}

// ReadStringBytes consumes a string as a borrowed view with capacity equal to length.
// It preserves bytes without UTF-8 validation or normalization. Mutating src changes
// the view; retaining it retains src's backing array. Copy before reusing that storage.
// Failures return nil and consume no bytes; use err to identify empty-string success.
func ReadStringBytes(src []byte) (value []byte, n int, err error) {
	if len(src) == 0 {
		return nil, 0, ErrTruncated
	}
	if src[0] >= 0xa0 && src[0] <= 0xbf {
		return readPayload(src, 1, uint64(src[0]&0x1f))
	}

	switch src[0] {
	case 0xd9:
		if len(src) < 2 {
			return nil, 0, ErrTruncated
		}
		return readPayload(src, 2, uint64(src[1]))
	case 0xda:
		if len(src) < 3 {
			return nil, 0, ErrTruncated
		}
		return readPayload(src, 3, uint64(binary.BigEndian.Uint16(src[1:3])))
	case 0xdb:
		if len(src) < 5 {
			return nil, 0, ErrTruncated
		}
		return readPayload(src, 5, uint64(binary.BigEndian.Uint32(src[1:5])))
	default:
		return nil, 0, readTypeError(src[0])
	}
}
