package messagepack

import "io"

func newStreamArrayReader(count uint32) (r *streamArrayReader) {
	r = &streamArrayReader{count: count}
	r.pending = AppendArrayHeader(r.scratch[:0], count)
	return r
}

// streamArrayReader generates each integer only when the decoder requests it.
type streamArrayReader struct {
	count      uint32
	next       uint64
	bytes      uint64
	reads      int
	maxRequest int
	scratch    [9]byte
	pending    []byte
}

func (r *streamArrayReader) Read(dst []byte) (n int, err error) {
	r.reads++
	if len(dst) > r.maxRequest {
		r.maxRequest = len(dst)
	}
	if len(dst) == 0 {
		return 0, nil
	}
	if len(r.pending) == 0 {
		if r.next == uint64(r.count) {
			return 0, io.EOF
		}
		r.pending = AppendUint64(r.scratch[:0], r.next)
		r.next++
	}
	n = copy(dst, r.pending)
	r.pending = r.pending[n:]
	r.bytes += uint64(n)
	return n, nil
}
