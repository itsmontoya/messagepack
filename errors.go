package messagepack

import "errors"

var (
	// ErrTruncated indicates empty input or an incomplete expected header or payload.
	ErrTruncated = errors.New("messagepack: truncated input")
	// ErrInvalidFormat indicates the reserved MessagePack tag 0xc1.
	ErrInvalidFormat = errors.New("messagepack: invalid format")
	// ErrUnsupportedType indicates an extension tag, including timestamp encodings.
	ErrUnsupportedType = errors.New("messagepack: unsupported type")
	// ErrTypeMismatch indicates a supported wire family rejected by the requested reader.
	ErrTypeMismatch = errors.New("messagepack: type mismatch")
	// ErrOverflow indicates an unrepresentable numeric value or an oversized append payload.
	ErrOverflow = errors.New("messagepack: overflow")
)

// readTypeError classifies a leading tag that the requested reader cannot accept.
func readTypeError(tag byte) (err error) {
	switch tag {
	case 0xc1:
		return ErrInvalidFormat
	case 0xc7, 0xc8, 0xc9, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8:
		return ErrUnsupportedType
	default:
		return ErrTypeMismatch
	}
}
