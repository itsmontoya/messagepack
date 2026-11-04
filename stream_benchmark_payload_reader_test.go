package messagepack

import (
	"bytes"
	"io"
	"testing"
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

// Reset selects raw or encoded payload generation and preserves read counts.
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

// Read emits pending header bytes before generating the requested payload prefix.
func (r *streamBenchmarkPayloadReader) Read(dst []byte) (n int, err error) {
	r.reads++
	n = copy(dst, r.pending)
	r.pending = r.pending[n:]
	dst = dst[n:]
	var size = len(dst)
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

func TestStreamBenchmarkPayloadSource(t *testing.T) {
	for _, testcase := range []struct {
		name     string
		isString bool
		encoded  bool
		wire     []byte
	}{
		{name: "raw", wire: []byte{'x', 'x', 'x'}},
		{name: "binary", encoded: true, wire: []byte{0xc4, 3, 'x', 'x', 'x'}},
		{name: "string", encoded: true, isString: true, wire: []byte{0xa3, 'x', 'x', 'x'}},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			for _, chunkSize := range []int{1, 8} {
				var (
					source streamBenchmarkPayloadReader
					actual []byte
					buffer [8]byte
					n      int
					err    error
				)
				source.Reset(3, testcase.isString, testcase.encoded)
				for {
					n, err = source.Read(buffer[:chunkSize])
					actual = append(actual, buffer[:n]...)
					if err == io.EOF {
						break
					}

					if err != nil {
						t.Fatal(err)
					}
				}
				if !bytes.Equal(actual, testcase.wire) {
					t.Fatalf("chunk %d: wire %x, want %x", chunkSize, actual, testcase.wire)
				}
			}
		})
	}
}
