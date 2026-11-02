package messagepack

import "io"

// decoderTestReader exposes fragmentation, simultaneous errors, and empty reads.
type decoderTestReader struct {
	data         []byte
	position     int
	maxRead      int
	endErr       error
	initialEmpty int
	emptyBetween int
	pendingEmpty int
	reads        int
	requests     []int
	closes       int
}

func (r *decoderTestReader) Read(dst []byte) (n int, err error) {
	if r == nil {
		dst[0] = 0xc3
		return 1, nil
	}
	r.reads++
	r.requests = append(r.requests, len(dst))
	if r.initialEmpty > 0 {
		r.initialEmpty--
		return 0, nil
	}
	if r.pendingEmpty > 0 {
		r.pendingEmpty--
		return 0, nil
	}
	if r.position == len(r.data) {
		if r.endErr != nil {
			return 0, r.endErr
		}
		return 0, io.EOF
	}
	n = len(dst)
	if r.maxRead > 0 && n > r.maxRead {
		n = r.maxRead
	}
	if n > len(r.data)-r.position {
		n = len(r.data) - r.position
	}
	copy(dst[:n], r.data[r.position:r.position+n])
	r.position += n
	r.pendingEmpty = r.emptyBetween
	if r.position == len(r.data) {
		return n, r.endErr
	}
	return n, nil
}

func (r *decoderTestReader) Close() (err error) {
	r.closes++
	return nil
}
