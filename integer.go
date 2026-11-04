package messagepack

import (
	"encoding/binary"
	"math"
)

// ReadInt64 accepts any integer encoding whose value fits int64.
// It leaves trailing bytes and src unchanged; failures return zero and consume nothing.
func ReadInt64(src []byte) (value int64, n int, err error) {
	if len(src) == 0 {
		return 0, 0, ErrTruncated
	}
	if src[0] <= 0x7f {
		return int64(src[0]), 1, nil
	}
	if src[0] >= 0xe0 {
		return int64(int8(src[0])), 1, nil
	}

	switch src[0] {
	case 0xcc:
		if len(src) < 2 {
			return 0, 0, ErrTruncated
		}
		return int64(src[1]), 2, nil
	case 0xcd:
		if len(src) < 3 {
			return 0, 0, ErrTruncated
		}
		return int64(binary.BigEndian.Uint16(src[1:3])), 3, nil
	case 0xce:
		if len(src) < 5 {
			return 0, 0, ErrTruncated
		}
		return int64(binary.BigEndian.Uint32(src[1:5])), 5, nil
	case 0xcf:
		if len(src) < 9 {
			return 0, 0, ErrTruncated
		}
		unsigned := binary.BigEndian.Uint64(src[1:9])
		if unsigned > math.MaxInt64 {
			return 0, 0, ErrOverflow
		}
		return int64(unsigned), 9, nil
	case 0xd0:
		if len(src) < 2 {
			return 0, 0, ErrTruncated
		}
		return int64(int8(src[1])), 2, nil
	case 0xd1:
		if len(src) < 3 {
			return 0, 0, ErrTruncated
		}
		return int64(int16(binary.BigEndian.Uint16(src[1:3]))), 3, nil
	case 0xd2:
		if len(src) < 5 {
			return 0, 0, ErrTruncated
		}
		return int64(int32(binary.BigEndian.Uint32(src[1:5]))), 5, nil
	case 0xd3:
		if len(src) < 9 {
			return 0, 0, ErrTruncated
		}
		return int64(binary.BigEndian.Uint64(src[1:9])), 9, nil
	default:
		return 0, 0, readTypeError(src[0])
	}
}

// ReadUint64 accepts any nonnegative integer encoding whose value fits uint64.
// It leaves trailing bytes and src unchanged; failures return zero and consume nothing.
func ReadUint64(src []byte) (value uint64, n int, err error) {
	if len(src) > 0 && src[0] == 0xcf {
		if len(src) < 9 {
			return 0, 0, ErrTruncated
		}
		return binary.BigEndian.Uint64(src[1:9]), 9, nil
	}

	var signed int64
	signed, n, err = ReadInt64(src)
	if err != nil {
		return 0, 0, err
	}
	if signed < 0 {
		return 0, 0, ErrOverflow
	}
	return uint64(signed), n, nil
}
