//go:build !goexperiment.simd || (!arm64 && !amd64)

package messagepack

import "io"

const numericSIMDEnabled = false

func decodeInt64sSIMD(_ io.Reader, _ []int64) (n int, err error) {
	return 0, nil
}

func decodeUint64sSIMD(_ io.Reader, _ []uint64) (n int, err error) {
	return 0, nil
}

func decodeFloat64sSIMD(_ io.Reader, _ []float64) (n int, err error) {
	return 0, nil
}
