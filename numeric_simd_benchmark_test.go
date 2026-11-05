//go:build goexperiment.simd && (arm64 || amd64)

package messagepack

import (
	"encoding/binary"
	"slices"
	"testing"
)

// BenchmarkNumericPairConversion isolates conversion after input is contiguous.
func BenchmarkNumericPairConversion(b *testing.B) {
	if !numericSIMDSupported() {
		b.Skip("SIMD instructions unavailable")
	}

	for _, mode := range []string{"Scalar", "SIMD"} {
		b.Run(mode, func(b *testing.B) {
			var (
				wire = make([]byte, 1024*9)
				dst  = make([]uint64, 1024)
				want = make([]uint64, 1024)
			)

			for index := range want {
				want[index] = uint64(index) * 0x01020304050607
				wire[index*9] = 0xcf
				binary.BigEndian.PutUint64(wire[index*9+1:], want[index])
			}
			b.SetBytes(int64(len(wire)))
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if mode == "SIMD" {
					for index := 0; index < len(dst); index += 2 {
						loadNumericPair(wire[index*9:]).Store(dst[index:])
					}
				} else {
					for index := 0; index < len(dst); index += 2 {
						dst[index] = binary.BigEndian.Uint64(wire[index*9+1:])
						dst[index+1] = binary.BigEndian.Uint64(wire[index*9+10:])
					}
				}
			}
			b.StopTimer()
			if !slices.Equal(dst, want) {
				b.Fatal("converted values differ")
			}
		})
	}
}

// BenchmarkNumericFixintConversion includes tag validation in both implementations.
func BenchmarkNumericFixintConversion(b *testing.B) {
	if !numericSIMDSupported() {
		b.Skip("SIMD instructions unavailable")
	}

	for _, mode := range []string{"Scalar", "SIMD"} {
		b.Run(mode, func(b *testing.B) {
			var (
				wire = make([]byte, 1024)
				dst  = make([]uint64, 1024)
			)

			for index := range wire {
				wire[index] = byte(index % 128)
			}
			b.SetBytes(int64(len(wire)))
			b.ReportAllocs()
			b.ResetTimer()
			for iteration := 0; iteration < b.N; iteration++ {
				if mode == "SIMD" {
					for index := 0; index < len(dst); index += 16 {
						decodePositiveUint64s(wire[index:], dst[index:])
					}
				} else {
					for index, value := range wire {
						if value > 0x7f {
							b.Fatal("input is not a positive fixint")
						}

						dst[index] = uint64(value)
					}
				}
			}
			b.StopTimer()
			for index, value := range dst {
				if value != uint64(wire[index]) {
					b.Fatal("converted fixints differ")
				}
			}
		})
	}
}
