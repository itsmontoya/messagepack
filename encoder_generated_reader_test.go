package messagepack

import "io"

// encoderGeneratedReader generates a deterministic payload without storing it.
type encoderGeneratedReader struct {
	length     uint64
	read       uint64
	reads      int
	maxRequest int
}

func (r *encoderGeneratedReader) Read(dst []byte) (n int, err error) {
	r.reads++
	if len(dst) > r.maxRequest {
		r.maxRequest = len(dst)
	}
	if r.read == r.length {
		return 0, io.EOF
	}
	n = len(dst)
	if uint64(n) > r.length-r.read {
		n = int(r.length - r.read)
	}
	for index := 0; index < n; index++ {
		dst[index] = byte(r.read + uint64(index))
	}
	r.read += uint64(n)
	return n, nil
}
