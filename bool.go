package messagepack

// AppendBool appends value while preserving dst's prefix and reusing its capacity.
func AppendBool(dst []byte, value bool) (out []byte) {
	if value {
		return append(dst, 0xc3)
	}
	return append(dst, 0xc2)
}

// ReadBool consumes one boolean, leaving trailing bytes for the caller.
// A failed read returns false, consumes no bytes, and leaves src unchanged.
func ReadBool(src []byte) (value bool, n int, err error) {
	if len(src) == 0 {
		return false, 0, ErrTruncated
	}
	switch src[0] {
	case 0xc2:
		return false, 1, nil
	case 0xc3:
		return true, 1, nil
	default:
		return false, 0, readTypeError(src[0])
	}
}
