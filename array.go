package messagepack

import "encoding/binary"

// AppendArrayHeader appends the smallest header for count elements.
// It preserves dst's prefix and reuses available capacity; growth may allocate.
func AppendArrayHeader(dst []byte, count uint32) (out []byte) {
	switch {
	case count <= 15:
		return append(dst, 0x90|byte(count))
	case count <= 65535:
		return binary.BigEndian.AppendUint16(append(dst, 0xdc), uint16(count))
	default:
		return binary.BigEndian.AppendUint32(append(dst, 0xdd), count)
	}
}

// ReadArrayHeader consumes only an array header and returns its element count.
// Success does not validate child values or require them to exist. It leaves src
// unchanged; failures return zero and consume no bytes.
func ReadArrayHeader(src []byte) (value uint32, n int, err error) {
	if len(src) == 0 {
		return 0, 0, ErrTruncated
	}
	if src[0] >= 0x90 && src[0] <= 0x9f {
		return uint32(src[0] & 0x0f), 1, nil
	}

	switch src[0] {
	case 0xdc:
		if len(src) < 3 {
			return 0, 0, ErrTruncated
		}
		return uint32(binary.BigEndian.Uint16(src[1:3])), 3, nil
	case 0xdd:
		if len(src) < 5 {
			return 0, 0, ErrTruncated
		}
		return binary.BigEndian.Uint32(src[1:5]), 5, nil
	default:
		return 0, 0, readTypeError(src[0])
	}
}
