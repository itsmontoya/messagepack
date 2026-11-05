package messagepack

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"testing"
)

// BenchmarkReaderInt64s compares scalar and batch calls on the same mixed widths.
func BenchmarkReaderInt64s(b *testing.B) {
	var (
		fragments = [][]byte{{0xff}, {0xd1, 0xff, 0x7f}, {0xd3, 0x80, 0, 0, 0, 0, 0, 0, 0}}
		values    = []int64{-1, -129, math.MinInt64}
	)

	for _, size := range []int{1, 2, 4, 8, 16, 64, 256, 1024} {
		for _, mode := range []string{"Scalar", "Batch"} {
			b.Run(fmt.Sprintf("Elements%d/%s", size, mode), func(b *testing.B) {
				var (
					wire []byte
					want = make([]int64, size)
					dst  = make([]int64, size)
					n    int
					err  error
				)

				for index := range want {
					wire = append(wire, fragments[index%len(fragments)]...)
					want[index] = values[index%len(values)]
				}
				var (
					source = bytes.NewReader(wire)
					r      = NewReader(source)
				)

				b.ReportAllocs()
				b.SetBytes(int64(len(wire)))
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					source.Reset(wire)
					if mode == "Batch" {
						n, err = r.DecodeInt64s(dst)
					} else {
						for n = 0; n < len(dst); n++ {
							if dst[n], err = r.DecodeInt64(); err != nil {
								b.Fatal(err)
							}
						}
					}

					if err != nil || n != size {
						b.Fatalf("decode = (%d, %v), want %d elements", n, err, size)
					}
				}
				b.StopTimer()
				if source.Len() != 0 || !slices.Equal(dst, want) {
					b.Fatal("decoded values or consumed bytes differ")
				}
			})
		}
	}
}

// BenchmarkReaderUint64s includes compact, wide, and signed integer encodings.
func BenchmarkReaderUint64s(b *testing.B) {
	var (
		fragments = [][]byte{{7}, {0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, {0xd1, 0, 1}}
		values    = []uint64{7, math.MaxUint64, 1}
	)

	for _, size := range []int{1, 2, 4, 8, 16, 64, 256, 1024} {
		for _, mode := range []string{"Scalar", "Batch"} {
			b.Run(fmt.Sprintf("Elements%d/%s", size, mode), func(b *testing.B) {
				var (
					wire []byte
					want = make([]uint64, size)
					dst  = make([]uint64, size)
					n    int
					err  error
				)

				for index := range want {
					wire = append(wire, fragments[index%len(fragments)]...)
					want[index] = values[index%len(values)]
				}
				var (
					source = bytes.NewReader(wire)
					r      = NewReader(source)
				)

				b.ReportAllocs()
				b.SetBytes(int64(len(wire)))
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					source.Reset(wire)
					if mode == "Batch" {
						n, err = r.DecodeUint64s(dst)
					} else {
						for n = 0; n < len(dst); n++ {
							if dst[n], err = r.DecodeUint64(); err != nil {
								b.Fatal(err)
							}
						}
					}

					if err != nil || n != size {
						b.Fatalf("decode = (%d, %v), want %d elements", n, err, size)
					}
				}
				b.StopTimer()
				if source.Len() != 0 || !slices.Equal(dst, want) {
					b.Fatal("decoded values or consumed bytes differ")
				}
			})
		}
	}
}

// BenchmarkReaderFloat64s includes both float64 and widened float32 encodings.
func BenchmarkReaderFloat64s(b *testing.B) {
	var (
		fragments = [][]byte{{0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}, {0xca, 0x40, 0x20, 0, 0}}
		values    = []float64{1.5, 2.5}
	)

	for _, size := range []int{1, 2, 4, 8, 16, 64, 256, 1024} {
		for _, mode := range []string{"Scalar", "Batch"} {
			b.Run(fmt.Sprintf("Elements%d/%s", size, mode), func(b *testing.B) {
				var (
					wire []byte
					want = make([]float64, size)
					dst  = make([]float64, size)
					n    int
					err  error
				)

				for index := range want {
					wire = append(wire, fragments[index%len(fragments)]...)
					want[index] = values[index%len(values)]
				}
				var (
					source = bytes.NewReader(wire)
					r      = NewReader(source)
				)

				b.ReportAllocs()
				b.SetBytes(int64(len(wire)))
				b.ResetTimer()
				for iteration := 0; iteration < b.N; iteration++ {
					source.Reset(wire)
					if mode == "Batch" {
						n, err = r.DecodeFloat64s(dst)
					} else {
						for n = 0; n < len(dst); n++ {
							if dst[n], err = r.DecodeFloat64(); err != nil {
								b.Fatal(err)
							}
						}
					}

					if err != nil || n != size {
						b.Fatalf("decode = (%d, %v), want %d elements", n, err, size)
					}
				}
				b.StopTimer()
				if source.Len() != 0 || !slices.Equal(dst, want) {
					b.Fatal("decoded values or consumed bytes differ")
				}
			})
		}
	}
}
