package messagepack

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"testing"
)

// BenchmarkReaderWideNumbers keeps the input path identical with and without SIMD.
func BenchmarkReaderWideNumbers(b *testing.B) {
	for _, kind := range []string{"Int64", "Uint64", "Float64", "PositiveInt64", "PositiveUint64"} {
		for _, size := range []int{1, 2, 4, 8, 16, 64, 256, 1024} {
			for _, mode := range []string{"Scalar", "Batch"} {
				b.Run(fmt.Sprintf("%s/Elements%d/%s", kind, size, mode), func(b *testing.B) {
					var (
						value  = uint64(0x0123456789abcdef)
						wire   = make([]byte, size*9)
						decode func() (int, error)
						check  func() bool
						n      int
						err    error
					)

					if kind == "PositiveInt64" || kind == "PositiveUint64" {
						wire = bytes.Repeat([]byte{7}, size)
						value = 7
					} else {
						for index := 0; index < size; index++ {
							wire[index*9] = 0xcf
							if kind == "Int64" {
								wire[index*9] = 0xd3
							} else if kind == "Float64" {
								wire[index*9] = 0xcb
							}

							binary.BigEndian.PutUint64(wire[index*9+1:], value)
						}
					}
					var (
						source = bytes.NewReader(wire)
						r      = NewReader(source)
					)

					switch kind {
					case "Int64", "PositiveInt64":
						var dst = make([]int64, size)
						decode = func() (completed int, decodeErr error) {
							if mode == "Batch" {
								return r.DecodeInt64s(dst)
							}

							for completed < len(dst) {
								if dst[completed], decodeErr = r.DecodeInt64(); decodeErr != nil {
									return completed, decodeErr
								}

								completed++
							}
							return completed, nil
						}
						check = func() (valid bool) {
							for _, number := range dst {
								if uint64(number) != value {
									return false
								}
							}
							return true
						}
					case "Uint64", "PositiveUint64":
						var dst = make([]uint64, size)
						decode = func() (completed int, decodeErr error) {
							if mode == "Batch" {
								return r.DecodeUint64s(dst)
							}

							for completed < len(dst) {
								if dst[completed], decodeErr = r.DecodeUint64(); decodeErr != nil {
									return completed, decodeErr
								}

								completed++
							}
							return completed, nil
						}
						check = func() (valid bool) {
							for _, number := range dst {
								if number != value {
									return false
								}
							}
							return true
						}
					case "Float64":
						var dst = make([]float64, size)
						decode = func() (completed int, decodeErr error) {
							if mode == "Batch" {
								return r.DecodeFloat64s(dst)
							}

							for completed < len(dst) {
								if dst[completed], decodeErr = r.DecodeFloat64(); decodeErr != nil {
									return completed, decodeErr
								}

								completed++
							}
							return completed, nil
						}
						check = func() (valid bool) {
							for _, number := range dst {
								if math.Float64bits(number) != value {
									return false
								}
							}
							return true
						}
					}
					b.ReportAllocs()
					b.SetBytes(int64(len(wire)))
					b.ResetTimer()
					for iteration := 0; iteration < b.N; iteration++ {
						source.Reset(wire)
						if n, err = decode(); err != nil || n != size {
							b.Fatalf("decode = (%d, %v), want %d values", n, err, size)
						}
					}
					b.StopTimer()
					if !check() || source.Len() != 0 {
						b.Fatal("decoded values or consumed input differ")
					}
				})
			}
		}
	}
}
