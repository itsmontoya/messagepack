package messagepack

import "errors"

var (
	// ErrInvalidFormat indicates the reserved MessagePack tag 0xc1.
	ErrInvalidFormat = errors.New("messagepack: invalid format")
	// ErrUnsupportedType indicates an extension tag or a value lacking the required codec hook.
	ErrUnsupportedType = errors.New("messagepack: unsupported type")
	// ErrTypeMismatch indicates a supported wire family rejected by the requested reader.
	ErrTypeMismatch = errors.New("messagepack: type mismatch")
	// ErrOverflow indicates an unrepresentable numeric value or an oversized payload.
	ErrOverflow = errors.New("messagepack: overflow")
	// ErrUninitialized indicates a stream codec without an attached reader or writer.
	ErrUninitialized = errors.New("messagepack: uninitialized stream")
	// ErrClosed indicates an Encoder or Decoder that has been closed.
	ErrClosed = errors.New("messagepack: closed codec")
	// ErrInvalidArgument indicates invalid limits or a required nil transfer interface.
	ErrInvalidArgument = errors.New("messagepack: invalid argument")
	// ErrLimitExceeded indicates a consumed value exceeds a configured decoder limit.
	ErrLimitExceeded = errors.New("messagepack: limit exceeded")
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
