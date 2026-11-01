package messagepack

// AppendNil appends nil while preserving dst's prefix and reusing its capacity.
func AppendNil(dst []byte) (out []byte) {
	return append(dst, 0xc0)
}

// ReadNil consumes one nil value, leaving trailing bytes for the caller.
// A failed read consumes no bytes and leaves src unchanged.
func ReadNil(src []byte) (n int, err error) {
	if len(src) == 0 {
		return 0, ErrTruncated
	}
	if src[0] != 0xc0 {
		return 0, readTypeError(src[0])
	}
	return 1, nil
}
