package messagepack

import "encoding/binary"

// AppendInt64 appends value in its smallest integer representation.
// Nonnegative values encode identically to AppendUint64; dst's prefix is preserved.
func AppendInt64(dst []byte, value int64) (out []byte) {
	if value >= 0 {
		return AppendUint64(dst, uint64(value))
	}
	switch {
	case value >= -32:
		return append(dst, byte(value))
	case value >= -128:
		return append(dst, 0xd0, byte(value))
	case value >= -32768:
		return binary.BigEndian.AppendUint16(append(dst, 0xd1), uint16(value))
	case value >= -2147483648:
		return binary.BigEndian.AppendUint32(append(dst, 0xd2), uint32(value))
	default:
		return binary.BigEndian.AppendUint64(append(dst, 0xd3), uint64(value))
	}
}

// AppendUint64 appends value in its smallest integer representation.
// It preserves dst's prefix and reuses available capacity; growth may allocate.
func AppendUint64(dst []byte, value uint64) (out []byte) {
	switch {
	case value <= 127:
		return append(dst, byte(value))
	case value <= 255:
		return append(dst, 0xcc, byte(value))
	case value <= 65535:
		return binary.BigEndian.AppendUint16(append(dst, 0xcd), uint16(value))
	case value <= 4294967295:
		return binary.BigEndian.AppendUint32(append(dst, 0xce), uint32(value))
	default:
		return binary.BigEndian.AppendUint64(append(dst, 0xcf), value)
	}
}
