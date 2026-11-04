package messagepack

import (
	"bytes"
	"io"
	"testing"
)

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

func TestStreamBenchmarkArraySource(t *testing.T) {
	for _, chunkSize := range []int{1, 8} {
		var (
			source = streamBenchmarkArrayReader{count: 3}
			actual []byte
			buffer [8]byte
			n      int
			err    error
		)
		source.Reset()
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
		if !bytes.Equal(actual, []byte{0x93, 0, 1, 2}) {
			t.Fatalf("chunk %d: wire %x, want 93000102", chunkSize, actual)
		}
	}
}
