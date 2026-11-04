package messagepack

import (
	"bufio"
	"fmt"
	"io"
	"testing"
)

var (
	streamBenchmarkWriterSink  *Writer
	streamBenchmarkDecoderSink *Decoder
)

func BenchmarkStreamPrimitives(b *testing.B) {
	var source = streamBenchmarkFixtureReader{data: []byte{
		0xc0, 0xc3, 0xd2, 0xff, 0xff, 0x7f, 0xff,
		0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xca, 0x3f, 0xc0, 0x00, 0x00,
		0xcb, 0x3f, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}}
	b.Run("Encode", func(b *testing.B) {
		benchmarkStreamRecordEncode(b, len(source.data), encodeStreamBenchmarkPrimitives)
	})
	b.Run("Decode", func(b *testing.B) {
		benchmarkStreamRecordDecode(b, len(source.data), &source, source.Reset, &source.reads, decodeStreamBenchmarkPrimitives)
	})
}

func BenchmarkStreamMessage(b *testing.B) {
	var (
		source  = streamBenchmarkFixtureReader{data: applicationMessageFixture()}
		message = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
			Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
	)
	b.Run("Encode", func(b *testing.B) {
		benchmarkStreamRecordEncode(b, len(source.data), func(e *Writer) (err error) {
			return encodeStreamingApplicationMessage(e, message)
		})
	})
	b.Run("Decode", func(b *testing.B) {
		benchmarkStreamRecordDecode(b, len(source.data), &source, source.Reset, &source.reads, func(d *Decoder) (err error) {
			benchmarkMessageSink, err = decodeStreamingApplicationMessage(d)
			return err
		})
	})
}

func BenchmarkStreamContainer(b *testing.B) {
	for _, count := range []uint32{1024, 65536} {
		b.Run(fmt.Sprintf("Elements%d", count), func(b *testing.B) {
			var (
				source = streamBenchmarkArrayReader{count: count}
				sizing streamBenchmarkWriter
				w      = NewWriter(&sizing)
			)
			if err := w.EncodeArrayHeader(count); err != nil {
				b.Fatal(err)
			}
			for index := uint64(0); index < uint64(count); index++ {
				if err := w.EncodeUint64(index); err != nil {
					b.Fatal(err)
				}
			}
			size := int(sizing.bytes)
			b.Run("Generator", func(b *testing.B) {
				var (
					buffer = make([]byte, payloadBufferSize)
					sink   streamBenchmarkWriter
					n      int
					err    error
				)
				source.reads = 0
				b.ReportAllocs()
				b.SetBytes(int64(size))
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					source.Reset()
					for remaining := size; remaining > 0; {
						chunk := buffer
						if remaining < len(chunk) {
							chunk = chunk[:remaining]
						}
						n, err = source.Read(chunk)
						if err != nil || n != len(chunk) {
							b.Fatalf("generate array = %d, %v", n, err)
						}
						if _, err = sink.Write(chunk[:n]); err != nil {
							b.Fatal(err)
						}
						remaining -= n
					}
				}
				b.StopTimer()
				streamBenchmarkCalls(b, source.reads, sink.writes)
			})
			b.Run("Encode", func(b *testing.B) {
				benchmarkStreamRecordEncode(b, size, func(e *Writer) (err error) {
					if err = e.EncodeArrayHeader(count); err != nil {
						return err
					}
					for index := uint64(0); index < uint64(count); index++ {
						if err = e.EncodeUint64(index); err != nil {
							return err
						}
					}
					return nil
				})
			})
			b.Run("Decode", func(b *testing.B) {
				benchmarkStreamRecordDecode(b, size, &source, source.Reset, &source.reads, func(d *Decoder) (err error) {
					var (
						actual uint32
						value  uint64
					)
					actual, err = d.DecodeArrayHeader()
					if err != nil {
						return err
					}
					if actual != count {
						return fmt.Errorf("benchmark: array count %d, want %d", actual, count)
					}
					for index := uint64(0); index < uint64(count); index++ {
						value, err = d.DecodeUint64()
						if err != nil {
							return err
						}
						if value != index {
							return fmt.Errorf("benchmark: element %d, want %d", value, index)
						}
					}
					benchmarkUintSink = value
					return nil
				})
			})
		})
	}
}

func benchmarkStreamRecordEncode(b *testing.B, size int, encode func(e *Writer) (err error)) {
	b.Helper()
	for _, buffered := range []bool{false, true} {
		for _, reuse := range []bool{false, true} {
			b.Run(streamBenchmarkMode(buffered, reuse), func(b *testing.B) {
				var (
					sink    streamBenchmarkWriter
					writer  io.Writer = &sink
					buffer  *bufio.Writer
					encoder *Writer
					err     error
				)
				if buffered {
					buffer = bufio.NewWriterSize(writer, 4096)
					writer = buffer
				}
				if reuse {
					encoder = NewWriter(writer)
					if err = encode(encoder); err != nil {
						b.Fatal(err)
					}
					if buffer != nil {
						if err = buffer.Flush(); err != nil {
							b.Fatal(err)
						}
					}
				}
				sink = streamBenchmarkWriter{}
				b.ReportAllocs()
				b.SetBytes(int64(size))
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					if reuse {
						encoder.Reset(writer)
					} else {
						encoder = NewWriter(writer)
						streamBenchmarkWriterSink = encoder
					}
					if err = encode(encoder); err != nil {
						b.Fatal(err)
					}
					if buffer != nil {
						if err = buffer.Flush(); err != nil {
							b.Fatal(err)
						}
					}
				}
				b.StopTimer()
				if sink.bytes != uint64(b.N)*uint64(size) {
					b.Fatalf("encoded %d bytes, want %d", sink.bytes, uint64(b.N)*uint64(size))
				}
				streamBenchmarkCalls(b, 0, sink.writes)
			})
		}
	}
}

func benchmarkStreamRecordDecode(b *testing.B, size int, source io.Reader, resetSource func(), reads *uint64, decode func(d *Decoder) (err error)) {
	b.Helper()
	for _, buffered := range []bool{false, true} {
		for _, reuse := range []bool{false, true} {
			b.Run(streamBenchmarkMode(buffered, reuse), func(b *testing.B) {
				var (
					reader  = source
					buffer  *bufio.Reader
					decoder *Decoder
					err     error
				)
				if buffered {
					buffer = bufio.NewReaderSize(reader, 4096)
					reader = buffer
				}
				if reuse {
					decoder = NewDecoder(reader)
				}
				*reads = 0
				b.ReportAllocs()
				b.SetBytes(int64(size))
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					resetSource()
					if buffer != nil {
						buffer.Reset(source)
					}
					if reuse {
						decoder.Reset(reader)
					} else {
						decoder = NewDecoder(reader)
						streamBenchmarkDecoderSink = decoder
					}
					if err = decode(decoder); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				streamBenchmarkCalls(b, *reads, 0)
			})
		}
	}
}

func streamBenchmarkMode(buffered bool, reuse bool) (name string) {
	name = "Unbuffered/New"
	if buffered {
		name = "Buffered4KiB/New"
	}
	if reuse {
		name = "Unbuffered/Reset"
		if buffered {
			name = "Buffered4KiB/Reset"
		}
	}
	return name
}

func streamBenchmarkCalls(b *testing.B, reads uint64, writes uint64) {
	b.ReportMetric(float64(reads)/float64(b.N), "readcalls/op")
	b.ReportMetric(float64(writes)/float64(b.N), "writecalls/op")
}

func encodeStreamBenchmarkPrimitives(e *Writer) (err error) {
	if err = e.EncodeNil(); err != nil {
		return err
	}
	if err = e.EncodeBool(true); err != nil {
		return err
	}
	if err = e.EncodeInt64(-32769); err != nil {
		return err
	}
	if err = e.EncodeUint64(^uint64(0)); err != nil {
		return err
	}
	if err = e.EncodeFloat32(1.5); err != nil {
		return err
	}
	return e.EncodeFloat64(1.5)
}

func decodeStreamBenchmarkPrimitives(d *Decoder) (err error) {
	if err = d.DecodeNil(); err != nil {
		return err
	}
	if benchmarkBoolSink, err = d.DecodeBool(); err != nil {
		return err
	}
	if benchmarkIntSink, err = d.DecodeInt64(); err != nil {
		return err
	}
	if benchmarkUintSink, err = d.DecodeUint64(); err != nil {
		return err
	}
	if benchmarkFloat32Sink, err = d.DecodeFloat32(); err != nil {
		return err
	}
	benchmarkFloat64Sink, err = d.DecodeFloat64()
	return err
}
