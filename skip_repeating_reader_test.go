package messagepack

import "io"

// skipRepeatingReader generates scalar children without retaining a container.
type skipRepeatingReader struct {
	value      byte
	remaining  uint64
	read       uint64
	maxRequest int
}

// Read generates scalar children without allocating their complete container.
func (r *skipRepeatingReader) Read(dst []byte) (n int, err error) {
	if len(dst) > r.maxRequest {
		r.maxRequest = len(dst)
	}

	if r.remaining == 0 {
		return 0, io.EOF
	}

	n = len(dst)
	if uint64(n) > r.remaining {
		n = int(r.remaining)
	}

	for index := 0; index < n; index++ {
		dst[index] = r.value
	}
	r.remaining -= uint64(n)
	r.read += uint64(n)
	return n, nil
}
