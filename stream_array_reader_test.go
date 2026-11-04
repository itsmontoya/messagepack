package messagepack

import (
	"bytes"
	"io"
)

func newStreamArrayReader(count uint32) (out *streamArrayReader) {
	var r streamArrayReader
	r.count = count
	r.buffer = *bytes.NewBuffer(r.scratch[:0])
	r.writer = NewWriter(&r.buffer)
	if err := r.writer.EncodeArrayHeader(count); err != nil {
		panic(err)
	}

	r.pending = r.buffer.Bytes()
	return &r
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
	buffer     bytes.Buffer
	writer     *Writer
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
		r.buffer.Reset()
		if err = r.writer.EncodeUint64(r.next); err != nil {
			return n, err
		}
		r.pending = r.buffer.Bytes()
		r.next++
	}
	n = copy(dst, r.pending)
	r.pending = r.pending[n:]
	r.bytes += uint64(n)
	return n, nil
}
