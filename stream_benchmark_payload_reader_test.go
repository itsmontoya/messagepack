package messagepack

import "io"

// streamBenchmarkPayloadReader generates x bytes and an optional wire header.
// Reset preserves cumulative call counts for benchmark reporting.
type streamBenchmarkPayloadReader struct {
	length   uint64
	position uint64
	reads    uint64
	header   [5]byte
	pending  []byte
}

func (r *streamBenchmarkPayloadReader) Reset(length uint64, isString bool, encoded bool) {
	r.length = length
	r.position = 0
	r.pending = nil
	if !encoded {
		return
	}
	// All harness sizes fit uint32; production header helpers are wire oracles.
	var err error
	if isString {
		r.pending, err = appendStringHeader(r.header[:0], length)
	} else {
		r.pending, err = appendBytesHeader(r.header[:0], length)
	}
	if err != nil {
		panic(err)
	}
}

func (r *streamBenchmarkPayloadReader) Read(dst []byte) (n int, err error) {
	r.reads++
	n = copy(dst, r.pending)
	r.pending = r.pending[n:]
	dst = dst[n:]
	size := len(dst)
	if uint64(size) > r.length-r.position {
		size = int(r.length - r.position)
	}
	for index := 0; index < size; index++ {
		dst[index] = 'x'
	}
	r.position += uint64(size)
	n += size
	if n == 0 && r.position == r.length && len(r.pending) == 0 {
		return 0, io.EOF
	}
	return n, nil
}
