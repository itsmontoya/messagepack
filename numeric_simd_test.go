//go:build goexperiment.simd && (arm64 || amd64)

package messagepack

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestNumericSIMDWideValues(t *testing.T) {
	if !numericSIMDSupported() {
		t.Skip("SIMD instructions unavailable")
	}

	for _, count := range []int{2, 3, 16, 17, 34, 35} {
		var (
			wire   = make([]byte, count*9)
			bits   = uint64(0x7ff0123456789abc)
			signed = make([]int64, count)
			uints  = make([]uint64, count)
			floats = make([]float64, count)
			wantN  = count - count%2
		)

		for index := 0; index < count; index++ {
			wire[index*9] = 0xd3
			binary.BigEndian.PutUint64(wire[index*9+1:], bits)
		}
		if n, err := decodeInt64sSIMD(bytes.NewReader(wire), signed); err != nil || n != wantN {
			t.Fatalf("SIMD signed count %d = (%d, %v), want %d", count, n, err, wantN)
		}

		for index := 0; index < count; index++ {
			wire[index*9] = 0xcf
		}
		if n, err := decodeUint64sSIMD(bytes.NewReader(wire), uints); err != nil || n != wantN {
			t.Fatalf("SIMD unsigned count %d = (%d, %v), want %d", count, n, err, wantN)
		}

		for index := 0; index < count; index++ {
			wire[index*9] = 0xcb
		}
		if n, err := decodeFloat64sSIMD(bytes.NewReader(wire), floats); err != nil || n != wantN {
			t.Fatalf("SIMD float count %d = (%d, %v), want %d", count, n, err, wantN)
		}

		for index := 0; index < wantN; index++ {
			if uint64(signed[index]) != bits || uints[index] != bits || math.Float64bits(floats[index]) != bits {
				t.Fatalf("SIMD count %d, element %d lost bits: %x, %x, %x", count, index,
					signed[index], uints[index], math.Float64bits(floats[index]))
			}
		}
	}
}

func TestNumericSIMDPositiveFixints(t *testing.T) {
	if !numericSIMDSupported() {
		t.Skip("SIMD instructions unavailable")
	}

	for _, invalid := range []int{-1, 0, 7, 15, 16, 17, 31, 127} {
		var (
			wire   = make([]byte, 128)
			signed [128]int64
			uints  [128]uint64
			wantN  = len(wire)
		)

		for index := range wire {
			wire[index] = byte(index)
		}
		if invalid >= 0 {
			wire[invalid] = 0xff
			wantN = invalid / 16 * 16
		}

		if n, err := decodeInt64sSIMD(bytes.NewReader(wire), signed[:]); err != nil || n != wantN {
			t.Fatalf("SIMD signed fixints, invalid %d = (%d, %v), want %d", invalid, n, err, wantN)
		}

		if n, err := decodeUint64sSIMD(bytes.NewReader(wire), uints[:]); err != nil || n != wantN {
			t.Fatalf("SIMD unsigned fixints, invalid %d = (%d, %v), want %d", invalid, n, err, wantN)
		}

		for index := 0; index < wantN; index++ {
			if signed[index] != int64(index) || uints[index] != uint64(index) {
				t.Fatalf("SIMD fixint %d = (%d, %d)", index, signed[index], uints[index])
			}
		}
		for index := wantN; index < len(wire); index++ {
			if signed[index] != 0 || uints[index] != 0 {
				t.Fatalf("SIMD fixint suffix changed at %d", index)
			}
		}
	}
}
