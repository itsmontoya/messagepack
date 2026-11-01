package messagepack

import (
	"bytes"
	"math"
	"strings"
	"testing"
)

func FuzzRoundTripInt64(f *testing.F) {
	for _, value := range []int64{math.MinInt64, -2147483649, -2147483648, -32769,
		-32768, -129, -128, -33, -32, -1, 0, 1, 127, 128, 255, 256, 65535,
		65536, 4294967295, 4294967296, math.MaxInt64} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value int64) {
		var (
			wire = AppendInt64(nil, value)
			got  int64
			n    int
			err  error
		)
		got, n, err = ReadInt64(wire)
		if err != nil || n != len(wire) || got != value {
			t.Fatalf("signed roundtrip %d: (%d, %d, %v)", value, got, n, err)
		}
	})
}

func FuzzRoundTripUint64(f *testing.F) {
	for _, value := range []uint64{0, 1, 127, 128, 255, 256, 65535, 65536,
		4294967295, 4294967296, math.MaxInt64, uint64(math.MaxInt64) + 1, math.MaxUint64} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value uint64) {
		var (
			wire = AppendUint64(nil, value)
			got  uint64
			n    int
			err  error
		)
		got, n, err = ReadUint64(wire)
		if err != nil || n != len(wire) || got != value {
			t.Fatalf("unsigned roundtrip %d: (%d, %d, %v)", value, got, n, err)
		}
	})
}

func FuzzRoundTripFloat32(f *testing.F) {
	for _, bits := range []uint32{0, 0x80000000, 1, 0x80000001, 0x3fc00000,
		0x7f7fffff, 0xff7fffff, 0x7f800000, 0xff800000, 0x7fc12345, 0x7f812345} {
		f.Add(bits)
	}
	f.Fuzz(func(t *testing.T, bits uint32) {
		var (
			wire = AppendFloat32(nil, math.Float32frombits(bits))
			got  float32
			n    int
			err  error
		)
		got, n, err = ReadFloat32(wire)
		if err != nil || n != len(wire) || math.Float32bits(got) != bits {
			t.Fatalf("float32 roundtrip %08x: (%08x, %d, %v)", bits, math.Float32bits(got), n, err)
		}
	})
}

func FuzzRoundTripFloat64(f *testing.F) {
	for _, bits := range []uint64{0, 0x8000000000000000, 1, 0x8000000000000001,
		0x3ff8000000000000, 0x7fefffffffffffff, 0xffefffffffffffff,
		0x7ff0000000000000, 0xfff0000000000000, 0x7ff8123456789abc, 0x7ff0123456789abc} {
		f.Add(bits)
	}
	f.Fuzz(func(t *testing.T, bits uint64) {
		var (
			wire = AppendFloat64(nil, math.Float64frombits(bits))
			got  float64
			n    int
			err  error
		)
		got, n, err = ReadFloat64(wire)
		if err != nil || n != len(wire) || math.Float64bits(got) != bits {
			t.Fatalf("float64 roundtrip %016x: (%016x, %d, %v)", bits, math.Float64bits(got), n, err)
		}
	})
}

func FuzzRoundTripString(f *testing.F) {
	for _, value := range []string{"", "x", "\x00é\xff", strings.Repeat("x", 31),
		strings.Repeat("x", 32), strings.Repeat("x", 255), strings.Repeat("x", 256),
		strings.Repeat("x", 4096)} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value string) {
		if len(value) > 4096 {
			return
		}
		var (
			wire  []byte
			owned string
			view  []byte
			n     int
			err   error
		)
		wire, err = AppendString(nil, value)
		if err != nil {
			t.Fatal(err)
		}
		owned, n, err = ReadString(wire)
		if err != nil || n != len(wire) || owned != value {
			t.Fatalf("owned string roundtrip length %d: (%q, %d, %v)", len(value), owned, n, err)
		}
		view, n, err = ReadStringBytes(wire)
		if err != nil || n != len(wire) || string(view) != value || cap(view) != len(view) {
			t.Fatalf("string view roundtrip length %d: (%x, %d, %v)", len(value), view, n, err)
		}
	})
}

func FuzzRoundTripBytes(f *testing.F) {
	for _, value := range [][]byte{nil, {}, {0x00, 0xc3, 0xa9, 0xff},
		bytes.Repeat([]byte{0x78}, 255), bytes.Repeat([]byte{0x78}, 256),
		bytes.Repeat([]byte{0x78}, 4096)} {
		f.Add(value)
	}
	f.Fuzz(func(t *testing.T, value []byte) {
		if len(value) > 4096 {
			return
		}
		var (
			wire []byte
			got  []byte
			n    int
			err  error
		)
		wire, err = AppendBytes(nil, value)
		if err != nil {
			t.Fatal(err)
		}
		got, n, err = ReadBytes(wire)
		if err != nil || n != len(wire) || !bytes.Equal(got, value) || cap(got) != len(got) {
			t.Fatalf("binary roundtrip length %d: (%x, %d, %v)", len(value), got, n, err)
		}
	})
}
