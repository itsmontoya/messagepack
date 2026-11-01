package messagepack

import "encoding/binary"

// AppendMapHeader appends the smallest header for count key/value pairs.
// It preserves dst's prefix and reuses available capacity; growth may allocate.
func AppendMapHeader(dst []byte, count uint32) (out []byte) {
	switch {
	case count <= 15:
		return append(dst, 0x80|byte(count))
	case count <= 65535:
		return binary.BigEndian.AppendUint16(append(dst, 0xde), uint16(count))
	default:
		return binary.BigEndian.AppendUint32(append(dst, 0xdf), count)
	}
}

// ReadMapHeader consumes only a map header and returns its key/value pair count.
// Success does not validate child values or require them to exist. It leaves src
// unchanged; failures return zero and consume no bytes.
func ReadMapHeader(src []byte) (value uint32, n int, err error) {
	if len(src) == 0 {
		return 0, 0, ErrTruncated
	}
	if src[0] >= 0x80 && src[0] <= 0x8f {
		return uint32(src[0] & 0x0f), 1, nil
	}

	switch src[0] {
	case 0xde:
		if len(src) < 3 {
			return 0, 0, ErrTruncated
		}
		return uint32(binary.BigEndian.Uint16(src[1:3])), 3, nil
	case 0xdf:
		if len(src) < 5 {
			return 0, 0, ErrTruncated
		}
		return binary.BigEndian.Uint32(src[1:5]), 5, nil
	default:
		return 0, 0, readTypeError(src[0])
	}
}
