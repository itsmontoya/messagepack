package messagepack

import "io"

// streamBenchmarkFixtureReader counts calls over only small literal fixtures.
type streamBenchmarkFixtureReader struct {
	data     []byte
	position int
	reads    uint64
}

func (r *streamBenchmarkFixtureReader) Reset() {
	r.position = 0
}

func (r *streamBenchmarkFixtureReader) Read(dst []byte) (n int, err error) {
	r.reads++
	if r.position == len(r.data) {
		return 0, io.EOF
	}
	n = copy(dst, r.data[r.position:])
	r.position += n
	return n, nil
}
