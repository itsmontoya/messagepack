//go:build goexperiment.simd

package messagepack

import "simd/archsimd"

func numericSIMDSupported() (supported bool) {
	return archsimd.X86.AVX()
}

func shuffleNumericBytes(value, indices archsimd.Uint8x16) (out archsimd.Uint8x16) {
	return value.PermuteOrZero(indices.BitsToInt8())
}
