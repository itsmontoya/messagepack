package messagepack

import (
	"bytes"
	"io"
)

// streamBenchmarkArrayReader fills requests incrementally using a reusable Writer.
type streamBenchmarkArrayReader struct {
	count   uint32
	next    uint64
	reads   uint64
	scratch [9]byte
	pending []byte
	buffer  bytes.Buffer
	writer  *Writer
}

// Reset starts a new array while preserving scratch and cumulative read counts.
func (r *streamBenchmarkArrayReader) Reset() {
	r.next = 0
	if r.writer == nil {
		r.buffer = *bytes.NewBuffer(r.scratch[:0])
		r.writer = NewWriter(&r.buffer)
	}

	r.buffer.Reset()
	if err := r.writer.EncodeArrayHeader(r.count); err != nil {
		panic(err)
	}

	r.pending = r.buffer.Bytes()
}

// Read fills requests incrementally without materializing the generated array.
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

			r.buffer.Reset()
			if err = r.writer.EncodeUint64(r.next); err != nil {
				return n, err
			}

			r.pending = r.buffer.Bytes()
			r.next++
		}

		size := copy(dst[n:], r.pending)
		r.pending = r.pending[size:]
		n += size
	}
	return n, nil
}
