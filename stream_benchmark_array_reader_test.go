package messagepack

import "io"

// streamBenchmarkArrayReader fills requests incrementally using nine-byte scratch.
type streamBenchmarkArrayReader struct {
	count   uint32
	next    uint64
	reads   uint64
	scratch [9]byte
	pending []byte
}

func (r *streamBenchmarkArrayReader) Reset() {
	r.next = 0
	r.pending = AppendArrayHeader(r.scratch[:0], r.count)
}

func (r *streamBenchmarkArrayReader) Read(dst []byte) (n int, err error) {
	r.reads++
	for n < len(dst) {
		if len(r.pending) == 0 {
			if r.next == uint64(r.count) {
				if n == 0 {
					return 0, io.EOF
				}
				return n, nil
			}
			r.pending = AppendUint64(r.scratch[:0], r.next)
			r.next++
		}
		size := copy(dst[n:], r.pending)
		r.pending = r.pending[size:]
		n += size
	}
	return n, nil
}
