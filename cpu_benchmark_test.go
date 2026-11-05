package messagepack

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// BenchmarkCPUTraversal compares warmed reflection with schema-specific traversal.
// Both paths use the same typed codec, key metadata, wire data, and owned results.
func BenchmarkCPUTraversal(b *testing.B) {
	tests := []struct {
		name    string
		nameLen int
		payload int
	}{
		{name: "Mixed", nameLen: 3, payload: 2},
		{name: "Binary64KiB", nameLen: 3, payload: 65536},
		{name: "String1KiB", nameLen: 1024, payload: 2},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			var (
				message = cpuBenchmarkMessage{ID: 7, Name: strings.Repeat("x", tt.nameLen),
					Active: true, Score: 1.5, Payload: bytes.Repeat([]byte{0x5a}, tt.payload), Samples: [2]int64{-1, 2}}
				plan = loadReflectionPlan(reflect.TypeFor[cpuBenchmarkMessage]())
				wire bytes.Buffer
				err  error
			)

			if err = NewEncoder(&wire).Encode(&message); err != nil {
				b.Fatal(err)
			}

			for _, mode := range []string{"Reflection", "Typed"} {
				b.Run(mode, func(b *testing.B) {
					b.Run("Encode", func(b *testing.B) {
						var (
							sink    streamBenchmarkWriter
							encoder = NewEncoder(&sink)
							writer  = NewWriter(&sink)
							encode  = func() (err error) { return encoder.Encode(&message) }
							output  bytes.Buffer
						)

						if mode == "Typed" {
							encode = func() (err error) { return message.encode(writer, plan.fields) }
						}

						if err = message.encode(NewWriter(&output), plan.fields); err != nil || !bytes.Equal(output.Bytes(), wire.Bytes()) {
							b.Fatalf("typed wire differs: %v", err)
						}

						if err = encode(); err != nil {
							b.Fatal(err)
						}

						sink = streamBenchmarkWriter{}
						b.ReportAllocs()
						b.SetBytes(int64(wire.Len()))
						b.ResetTimer()
						for index := 0; index < b.N; index++ {
							if err = encode(); err != nil {
								b.Fatal(err)
							}
						}
						b.StopTimer()
						b.ReportMetric(float64(sink.writes)/float64(b.N), "writecalls/op")
						if sink.bytes != uint64(b.N)*uint64(wire.Len()) {
							b.Fatal("encoded byte count differs")
						}
					})
					b.Run("Decode", func(b *testing.B) {
						var (
							input   = bytes.NewReader(wire.Bytes())
							decoder = NewDecoder(input)
							reader  = NewReader(input)
							target  cpuBenchmarkMessage
							decode  = func() (err error) { return decoder.Decode(&target) }
						)

						if mode == "Typed" {
							decode = func() (err error) { return target.decode(reader, plan.fieldsByName) }
						}

						if err = decode(); err != nil || !reflect.DeepEqual(target, message) {
							b.Fatalf("decoded value differs: %v", err)
						}

						b.ReportAllocs()
						b.SetBytes(int64(wire.Len()))
						b.ResetTimer()
						for index := 0; index < b.N; index++ {
							input.Reset(wire.Bytes())
							if err = decode(); err != nil {
								b.Fatal(err)
							}
						}
						b.StopTimer()
						if input.Len() != 0 || !reflect.DeepEqual(target, message) {
							b.Fatal("decoded value or consumed byte count differs")
						}
					})
				})
			}
		})
	}
}
