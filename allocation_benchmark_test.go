package messagepack

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkAllocationStringEncode(b *testing.B) {
	for _, length := range []int{0, 1, 8, 9, 10, 31, 32, 1024, 32768, 32769} {
		b.Run(fmt.Sprintf("Bytes%d", length), func(b *testing.B) {
			var (
				value = strings.Repeat("x", length)
				sink  streamBenchmarkWriter
				w     = NewWriter(&sink)
			)

			if err := w.EncodeString(value); err != nil {
				b.Fatal(err)
			}

			benchmarkStreamRecordEncode(b, int(sink.bytes), func(writer *Writer) (err error) {
				return writer.EncodeString(value)
			})
		})
	}
}

func BenchmarkAllocationOwnedSmall(b *testing.B) {
	for _, length := range []uint64{0, 1, 8, 31, 32, 256} {
		b.Run(fmt.Sprintf("Bytes%d", length), func(b *testing.B) {
			for _, isString := range []bool{false, true} {
				var name = "Binary"
				if isString {
					name = "String"
				}

				b.Run(name, func(b *testing.B) {
					var (
						source streamBenchmarkPayloadReader
						r      = NewReader(&source)
						err    error
					)

					source.Reset(length, isString, true)
					b.ReportAllocs()
					b.SetBytes(int64(length) + int64(len(source.pending)))
					b.ResetTimer()
					for index := 0; index < b.N; index++ {
						source.Reset(length, isString, true)
						r.Reset(&source)
						if isString {
							benchmarkStringSink, err = r.DecodeString()
						} else {
							benchmarkBytesSink, err = r.DecodeBytes()
						}

						if err != nil {
							b.Fatal(err)
						}
					}
					b.StopTimer()
					if source.position != length || len(source.pending) != 0 {
						b.Fatal("owned read did not consume exactly its payload")
					}
				})
			}
		})
	}
}

func BenchmarkAllocationHooks(b *testing.B) {
	for _, noIO := range []bool{true, false} {
		var name = "Uint64"
		if noIO {
			name = "NoIO"
		}

		b.Run(name, func(b *testing.B) {
			b.Run("Encode", func(b *testing.B) {
				var (
					sink   streamBenchmarkWriter
					e      = NewEncoder(&sink)
					value  = allocationBenchmarkValue{value: 42}
					noop   allocationBenchmarkNoop
					target Encodee = &value
				)

				if noIO {
					target = &noop
				}

				// Exercise the hook before timing to exclude first-use work.
				if err := e.Encode(target); err != nil {
					b.Fatal(err)
				}

				sink = streamBenchmarkWriter{}
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					if err := e.Encode(target); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				if !noIO && sink.bytes != uint64(b.N) {
					b.Fatalf("encoded %d bytes, want %d", sink.bytes, b.N)
				}
			})
			b.Run("Decode", func(b *testing.B) {
				var (
					source = streamBenchmarkFixtureReader{data: []byte{42}}
					d      = NewDecoder(&source)
					value  allocationBenchmarkValue
					noop   allocationBenchmarkNoop
					target Decodee = &value
				)

				if noIO {
					target = &noop
				}

				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					source.Reset()
					if err := d.Decode(target); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				if !noIO && (value.value != 42 || source.position != 1) {
					b.Fatalf("decoded %d at position %d", value.value, source.position)
				}
			})
		})
	}
}

func BenchmarkAllocationMessageHooks(b *testing.B) {
	var message = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
		Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}

	b.Run("Encode", func(b *testing.B) {
		var (
			sink streamBenchmarkWriter
			e    = NewEncoder(&sink)
		)

		if err := e.Encode(&message); err != nil {
			b.Fatal(err)
		}

		sink = streamBenchmarkWriter{}
		b.ReportAllocs()
		b.SetBytes(int64(len(applicationMessageFixture())))
		b.ResetTimer()
		for index := 0; index < b.N; index++ {
			if err := e.Encode(&message); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		if sink.bytes != uint64(b.N)*uint64(len(applicationMessageFixture())) {
			b.Fatalf("encoded %d bytes for %d messages", sink.bytes, b.N)
		}
	})
	b.Run("Decode", func(b *testing.B) {
		var (
			source = streamBenchmarkFixtureReader{data: applicationMessageFixture()}
			d      = NewDecoder(&source)
			value  applicationMessage
		)

		b.ReportAllocs()
		b.SetBytes(int64(len(source.data)))
		b.ResetTimer()
		for index := 0; index < b.N; index++ {
			source.Reset()
			if err := d.Decode(&value); err != nil {
				b.Fatal(err)
			}

			benchmarkMessageSink = value
		}
		b.StopTimer()
		if source.position != len(source.data) {
			b.Fatal("hook did not consume exactly its message")
		}
	})
}

func BenchmarkAllocationSkip(b *testing.B) {
	for _, testcase := range []struct {
		name string
		wire []byte
	}{
		{name: "Scalar", wire: []byte{42}},
		{name: "String9", wire: []byte{0xa9, '1', '2', '3', '4', '5', '6', '7', '8', '9'}},
		{name: "String10", wire: []byte{0xaa, '1', '2', '3', '4', '5', '6', '7', '8', '9', '0'}},
		{name: "Binary9", wire: []byte{0xc4, 9, '1', '2', '3', '4', '5', '6', '7', '8', '9'}},
		{name: "Binary10", wire: []byte{0xc4, 10, '1', '2', '3', '4', '5', '6', '7', '8', '9', '0'}},
		{name: "Message", wire: applicationMessageFixture()},
		{name: "Depth64", wire: append([]byte(strings.Repeat("\x91", 64)), 0xc0)},
	} {
		b.Run(testcase.name, func(b *testing.B) {
			var source = streamBenchmarkFixtureReader{data: testcase.wire}
			benchmarkStreamRecordDecode(b, len(testcase.wire), &source, source.Reset, &source.reads, (*Reader).Skip)
		})
	}
}
