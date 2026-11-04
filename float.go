package messagepack

import (
	"encoding/binary"
	"math"
)

// ReadFloat32 consumes only float32 encodings and preserves their bits.
// It leaves src unchanged; failures return zero and consume no bytes.
func ReadFloat32(src []byte) (value float32, n int, err error) {
	if len(src) == 0 {
		return 0, 0, ErrTruncated
	}
	if src[0] != 0xca {
		return 0, 0, readTypeError(src[0])
	}
	if len(src) < 5 {
		return 0, 0, ErrTruncated
	}
	return math.Float32frombits(binary.BigEndian.Uint32(src[1:5])), 5, nil
}

// ReadFloat64 consumes float64 unchanged or widens float32 with Go's conversion.
// Widened NaN payload bits are unspecified. Failures return zero and consume no bytes.
func ReadFloat64(src []byte) (value float64, n int, err error) {
	if len(src) == 0 {
		return 0, 0, ErrTruncated
	}
	switch src[0] {
	case 0xca:
		if len(src) < 5 {
			return 0, 0, ErrTruncated
		}
		return float64(math.Float32frombits(binary.BigEndian.Uint32(src[1:5]))), 5, nil
	case 0xcb:
		if len(src) < 9 {
			return 0, 0, ErrTruncated
		}
		return math.Float64frombits(binary.BigEndian.Uint64(src[1:9])), 9, nil
	default:
		return 0, 0, readTypeError(src[0])
	}
}
