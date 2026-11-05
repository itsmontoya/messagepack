package messagepack

import (
	"bytes"
	"testing"
)

func BenchmarkReflectionStruct(b *testing.B) {
	tests := []struct {
		name  string
		value any
	}{
		{
			name:  "SingleField",
			value: &struct{ ID uint64 }{ID: 42},
		},
		{
			name: "Scalars",
			value: &struct {
				ID     uint64
				Count  int64
				Active bool
				Ratio  float64
			}{ID: 42, Count: -3, Active: true, Ratio: 1.5},
		},
		{
			name: "Mixed",
			value: &struct {
				ID      uint64
				Name    string
				Active  bool
				Score   float64
				Payload []byte
				Samples [2]int64
			}{ID: 7, Name: "Ada", Active: true, Score: 1.5, Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}},
		},
		{
			name: "Nested",
			value: &struct {
				Left  struct{ ID uint64 }
				Right struct{ Active bool }
			}{Left: struct{ ID uint64 }{ID: 42}, Right: struct{ Active bool }{Active: true}},
		},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			var (
				output bytes.Buffer
				err    error
			)

			if err = NewEncoder(&output).Encode(tt.value); err != nil {
				b.Fatal(err)
			}

			b.Run("Encode", func(b *testing.B) {
				var (
					sink streamBenchmarkWriter
					e    = NewEncoder(&sink)
				)

				if err = e.Encode(tt.value); err != nil {
					b.Fatal(err)
				}

				sink = streamBenchmarkWriter{}
				b.ReportAllocs()
				b.SetBytes(int64(output.Len()))
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					if err = e.Encode(tt.value); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				if sink.bytes != uint64(b.N)*uint64(output.Len()) {
					b.Fatalf("encoded %d bytes for %d structs", sink.bytes, b.N)
				}
			})
			b.Run("Decode", func(b *testing.B) {
				var (
					source = streamBenchmarkFixtureReader{data: output.Bytes()}
					d      = NewDecoder(&source)
				)

				if err = d.Decode(tt.value); err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()
				b.SetBytes(int64(output.Len()))
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					source.Reset()
					if err = d.Decode(tt.value); err != nil {
						b.Fatal(err)
					}
				}
				b.StopTimer()
				if source.position != output.Len() {
					b.Fatal("decode did not consume exactly one struct")
				}
			})
		})
	}
}
