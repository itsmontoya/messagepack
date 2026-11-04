package messagepack

import (
	"bytes"
	"io"
)

// streamBenchmarkPayloadReader generates x bytes and an optional wire header.
// Reset preserves cumulative call counts for benchmark reporting.
type streamBenchmarkPayloadReader struct {
	length   uint64
	position uint64
	reads    uint64
	header   [5]byte
	pending  []byte
	buffer   bytes.Buffer
	writer   *Writer
}

func (r *streamBenchmarkPayloadReader) Reset(length uint64, isString bool, encoded bool) {
	r.length = length
	r.position = 0
	r.pending = nil
	if !encoded {
		return
	}
	if r.writer == nil {
		r.buffer = *bytes.NewBuffer(r.header[:0])
		r.writer = NewWriter(&r.buffer)
	}
	r.buffer.Reset()
	if err := r.writer.writePayloadHeader("benchmark header", length, isString); err != nil {
		panic(err)
	}
	r.pending = r.buffer.Bytes()
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
