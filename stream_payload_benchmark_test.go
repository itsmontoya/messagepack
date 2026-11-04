package messagepack

import (
	"bufio"
	"io"
	"testing"
)

var streamBenchmarkSizes = []struct {
	name   string
	length uint64
}{
	{name: "1KiB", length: 1024},
	{name: "64KiB", length: 65536},
	{name: "8MiB", length: 8388608},
}

func BenchmarkStreamTransfer(b *testing.B) {
	for _, size := range streamBenchmarkSizes {
		b.Run(size.name, func(b *testing.B) {
			for _, operation := range []struct {
				name     string
				encode   bool
				isString bool
			}{
				{name: "EncodeBytes", encode: true},
				{name: "EncodeString", encode: true, isString: true},
				{name: "DecodeBytes"},
				{name: "DecodeString", isString: true},
			} {
				b.Run(operation.name, func(b *testing.B) {
					for _, buffered := range []bool{false, true} {
						for _, reuse := range []bool{false, true} {
							b.Run(streamBenchmarkMode(buffered, reuse), func(b *testing.B) {
								benchmarkStreamTransfer(b, size.length, operation.encode, operation.isString, buffered, reuse)
							})
						}
					}
				})
			}
		})
	}
}

func benchmarkStreamTransfer(b *testing.B, length uint64, encode bool, isString bool, buffered bool, reuse bool) {
	b.Helper()
	var (
		source       streamBenchmarkPayloadReader
		sink         streamBenchmarkWriter
		reader       io.Reader = &source
		writer       io.Writer = &sink
		readerBuffer *bufio.Reader
		writerBuffer *bufio.Writer
		encoder      *Writer
		decoder      *Decoder
		n            uint64
		err          error
		header       [5]byte
		wireHeader   []byte
		transfer     func() (n uint64, err error)
	)
	if isString {
		wireHeader, err = appendStringHeader(header[:0], length)
	} else {
		wireHeader, err = appendBytesHeader(header[:0], length)
	}
	if err != nil {
		b.Fatal(err)
	}
	if buffered {
		readerBuffer = bufio.NewReaderSize(reader, 4096)
		writerBuffer = bufio.NewWriterSize(writer, 4096)
		reader = readerBuffer
		writer = writerBuffer
	}
	encoder = NewWriter(writer)
	decoder = NewDecoder(reader)
	if encode {
		transfer = func() (n uint64, err error) { return encoder.EncodeBytesFrom(length, reader) }
		if isString {
			transfer = func() (n uint64, err error) { return encoder.EncodeStringFrom(length, reader) }
		}
	} else {
		transfer = func() (n uint64, err error) { return decoder.DecodeBytesTo(writer) }
		if isString {
			transfer = func() (n uint64, err error) { return decoder.DecodeStringTo(writer) }
		}
	}
	if reuse {
		source.Reset(length, isString, !encode)
		if n, err = transfer(); err != nil || n != length {
			b.Fatalf("warm transfer = %d, %v; want %d, nil", n, err, length)
		}
		if writerBuffer != nil {
			if err = writerBuffer.Flush(); err != nil {
				b.Fatal(err)
			}
		}
	}
	source.reads = 0
	sink = streamBenchmarkWriter{}
	b.ReportAllocs()
	b.SetBytes(int64(length) + int64(len(wireHeader)))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		source.Reset(length, isString, !encode)
		if readerBuffer != nil {
			readerBuffer.Reset(&source)
		}
		if encode {
			if reuse {
				encoder.Reset(writer)
			} else {
				encoder = NewWriter(writer)
				streamBenchmarkWriterSink = encoder
			}
		} else {
			if reuse {
				decoder.Reset(reader)
			} else {
				decoder = NewDecoder(reader)
				streamBenchmarkDecoderSink = decoder
			}
		}
		if n, err = transfer(); err != nil || n != length {
			b.Fatalf("transfer = %d, %v; want %d, nil", n, err, length)
		}
		if writerBuffer != nil {
			if err = writerBuffer.Flush(); err != nil {
				b.Fatal(err)
			}
		}
	}
	b.StopTimer()
	expected := uint64(b.N) * length
	if encode {
		expected += uint64(b.N) * uint64(len(wireHeader))
	}
	if sink.bytes != expected || source.position != length || len(source.pending) != 0 {
		b.Fatalf("transfer boundary: wrote %d, want %d; source position %d", sink.bytes, expected, source.position)
	}
	streamBenchmarkCalls(b, source.reads, sink.writes)
}

func BenchmarkStreamOwned(b *testing.B) {
	for _, size := range streamBenchmarkSizes {
		b.Run(size.name, func(b *testing.B) {
			for _, isString := range []bool{false, true} {
				name := "Bytes"
				if isString {
					name = "String"
				}
				b.Run(name, func(b *testing.B) {
					var (
						source  streamBenchmarkPayloadReader
						decoder = NewDecoder(&source)
						err     error
						header  [5]byte
						wire    []byte
					)
					if isString {
						wire, err = appendStringHeader(header[:0], size.length)
					} else {
						wire, err = appendBytesHeader(header[:0], size.length)
					}
					if err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					b.SetBytes(int64(size.length) + int64(len(wire)))
					b.ResetTimer()
					for index := 0; index < b.N; index++ {
						source.Reset(size.length, isString, true)
						decoder.Reset(&source)
						if isString {
							benchmarkStringSink, err = decoder.DecodeString()
						} else {
							benchmarkBytesSink, err = decoder.DecodeBytes()
						}
						if err != nil {
							b.Fatal(err)
						}
					}
					b.StopTimer()
					if source.position != size.length || len(source.pending) != 0 {
						b.Fatal("owned read did not consume exactly its payload")
					}
					streamBenchmarkCalls(b, source.reads, 0)
				})
			}
		})
	}
}

// BenchmarkStreamGenerator measures the payload-generation harness separately.
func BenchmarkStreamGenerator(b *testing.B) {
	for _, size := range streamBenchmarkSizes {
		b.Run(size.name, func(b *testing.B) {
			var (
				source streamBenchmarkPayloadReader
				sink   streamBenchmarkWriter
				buffer = make([]byte, payloadBufferSize)
				n      int
				err    error
			)
			b.ReportAllocs()
			b.SetBytes(int64(size.length))
			b.ResetTimer()
			for index := 0; index < b.N; index++ {
				source.Reset(size.length, false, false)
				for remaining := size.length; remaining > 0; {
					chunk := buffer
					if remaining < uint64(len(chunk)) {
						chunk = chunk[:int(remaining)]
					}
					n, err = source.Read(chunk)
					if err != nil || n != len(chunk) {
						b.Fatalf("generate = %d, %v", n, err)
					}
					if _, err = sink.Write(chunk[:n]); err != nil {
						b.Fatal(err)
					}
					remaining -= uint64(n)
				}
			}
			b.StopTimer()
			streamBenchmarkCalls(b, source.reads, sink.writes)
		})
	}
}
