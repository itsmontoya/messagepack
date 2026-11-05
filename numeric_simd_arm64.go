//go:build goexperiment.simd

package messagepack

import "simd/archsimd"

func numericSIMDSupported() (supported bool) {
	return true
}

func shuffleNumericBytes(value, indices archsimd.Uint8x16) (out archsimd.Uint8x16) {
	return value.LookupOrZero(indices)
}
