package messagepack

import "encoding/binary"

// ReadBytes consumes binary as a borrowed view with capacity equal to length.
// Mutating src changes the view; retaining it retains src's backing array. Copy
// before reusing that storage. Failures return nil and consume no bytes; use err
// to identify empty-binary success. The reader never modifies src.
func ReadBytes(src []byte) (value []byte, n int, err error) {
	if len(src) == 0 {
		return nil, 0, ErrTruncated
	}

	switch src[0] {
	case 0xc4:
		if len(src) < 2 {
			return nil, 0, ErrTruncated
		}
		return readPayload(src, 2, uint64(src[1]))
	case 0xc5:
		if len(src) < 3 {
			return nil, 0, ErrTruncated
		}
		return readPayload(src, 3, uint64(binary.BigEndian.Uint16(src[1:3])))
	case 0xc6:
		if len(src) < 5 {
			return nil, 0, ErrTruncated
		}
		return readPayload(src, 5, uint64(binary.BigEndian.Uint32(src[1:5])))
	default:
		return nil, 0, readTypeError(src[0])
	}
}

// readPayload checks a complete header's length before converting it to int.
func readPayload(src []byte, header int, length uint64) (value []byte, n int, err error) {
	if length > uint64(len(src)-header) {
		return nil, 0, ErrTruncated
	}
	end := header + int(length)
	return src[header:end:end], end, nil
}
