package messagepack

import "encoding/binary"

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
