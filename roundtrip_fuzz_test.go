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
			output bytes.Buffer
			wire   []byte
			got    int64
			n      int
			err    error
		)
		if err = NewWriter(&output).EncodeInt64(value); err != nil {
			t.Fatal(err)
		}
		wire = output.Bytes()
		sourceInt64 := bytes.NewReader(wire)
		got, err = NewReader(sourceInt64).DecodeInt64()
		n = len(wire) - sourceInt64.Len()
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
			output bytes.Buffer
			wire   []byte
			got    uint64
			n      int
			err    error
		)
		if err = NewWriter(&output).EncodeUint64(value); err != nil {
			t.Fatal(err)
		}
		wire = output.Bytes()
		sourceUint64 := bytes.NewReader(wire)
		got, err = NewReader(sourceUint64).DecodeUint64()
		n = len(wire) - sourceUint64.Len()
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
			output bytes.Buffer
			wire   []byte
			got    float32
			n      int
			err    error
		)
		if err = NewWriter(&output).EncodeFloat32(math.Float32frombits(bits)); err != nil {
			t.Fatal(err)
		}
		wire = output.Bytes()
		sourceFloat32 := bytes.NewReader(wire)
		got, err = NewReader(sourceFloat32).DecodeFloat32()
		n = len(wire) - sourceFloat32.Len()
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
			output bytes.Buffer
			wire   []byte
			got    float64
			n      int
			err    error
		)
		if err = NewWriter(&output).EncodeFloat64(math.Float64frombits(bits)); err != nil {
			t.Fatal(err)
		}
		wire = output.Bytes()
		sourceFloat64 := bytes.NewReader(wire)
		got, err = NewReader(sourceFloat64).DecodeFloat64()
		n = len(wire) - sourceFloat64.Len()
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
			output bytes.Buffer
			wire   []byte
			owned  string
			n      int
			err    error
		)
		err = NewWriter(&output).EncodeString(value)
		if err != nil {
			t.Fatal(err)
		}
		wire = output.Bytes()
		sourceString := bytes.NewReader(wire)
		owned, err = NewReader(sourceString).DecodeString()
		n = len(wire) - sourceString.Len()
		if err != nil || n != len(wire) || owned != value {
			t.Fatalf("owned string roundtrip length %d: (%q, %d, %v)", len(value), owned, n, err)
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
			output bytes.Buffer
			wire   []byte
			got    []byte
			n      int
			err    error
		)
		err = NewWriter(&output).EncodeBytes(value)
		if err != nil {
			t.Fatal(err)
		}
		wire = output.Bytes()
		sourceBytes := bytes.NewReader(wire)
		got, err = NewReader(sourceBytes).DecodeBytes()
		n = len(wire) - sourceBytes.Len()
		if err != nil || n != len(wire) || !bytes.Equal(got, value) || cap(got) != len(got) {
			t.Fatalf("binary roundtrip length %d: (%x, %d, %v)", len(value), got, n, err)
		}
	})
}
