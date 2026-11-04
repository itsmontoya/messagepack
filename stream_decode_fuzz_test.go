package messagepack

import (
	"bytes"
	"io"
	"math"
	"testing"
)

func FuzzStreamTypedDecode(f *testing.F) {
	for _, wire := range streamFuzzSeeds() {
		for kind := uint8(0); kind < 10; kind++ {
			f.Add(kind, wire, []byte{0, 2, 63}, uint8(0), uint16(4095), uint8(0))
			f.Add(kind, wire, []byte{0}, uint8(0), uint16(31), uint8(1))
			for length := 0; length < len(wire); length++ {
				f.Add(kind, wire[:length], []byte{0}, uint8(99), uint16(31), uint8(0))
			}
		}
		f.Add(uint8(6), append(append([]byte(nil), wire...), 0xc3, 0xc1), []byte{1}, uint8(100), uint16(1), uint8(2))
	}
	f.Fuzz(func(t *testing.T, kind uint8, src, fragments []byte, empty uint8, capSeed uint16, mode uint8) {
		if len(src) > streamFuzzInputLimit || len(fragments) > 32 {
			t.Skip()
		}
		var (
			before = append([]byte(nil), src...)
			r      = &streamFuzzReader{data: src, fragments: fragments, empty: int(empty) % 102,
				emptyBetween: int(empty) % 3, endErr: io.EOF, dataError: true}
			d     = NewReader(r)
			limit = uint64(capSeed)%4096 + 1
		)
		if mode%3 != 0 {
			r.endErr = errStreamFuzzSource
			r.dataError = mode%3 == 1
		}
		if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: limit}); err != nil {
			t.Fatal(err)
		}
		plain := bytes.NewReader(src)
		baseline := NewReader(plain)
		if err := baseline.SetLimits(ReaderLimits{MaxPayloadBytes: limit}); err != nil {
			t.Fatal(err)
		}
		want, got := checkStreamFuzzTyped(t, d, baseline, kind%10)
		n := len(src) - plain.Len()
		want = streamFuzzError(want, len(src))
		if r.noProgress {
			want = io.ErrNoProgress
		}
		if r.deliveredError == errStreamFuzzSource {
			want = errStreamFuzzSource
		}
		requireStreamFuzzError(t, got, want)
		requireStreamFuzzInput(t, src, before, r, d)
		if got == nil {
			if r.position != n || n <= 0 || n > len(src) {
				t.Fatalf("typed success consumed %d bytes, contiguous Reader consumed %d of %d", r.position, n, len(src))
			}
			return
		}
		requireStreamFuzzTerminal(t, d, r, got)
	})
}

// Each case keeps its actual typed value, avoiding an any/reflection value model.
func checkStreamFuzzTyped(t *testing.T, d, baseline *Reader, kind uint8) (want, got error) {
	t.Helper()
	switch kind {
	case 0:
		want = baseline.DecodeNil()
		got = d.DecodeNil()
	case 1:
		var value, expected bool
		expected, want = baseline.DecodeBool()
		value, got = d.DecodeBool()
		if got == nil && value != expected || got != nil && value {
			t.Fatalf("bool result = %v, contiguous %v, error %v", value, expected, got)
		}
	case 2:
		var value, expected int64
		expected, want = baseline.DecodeInt64()
		value, got = d.DecodeInt64()
		if got == nil && value != expected || got != nil && value != 0 {
			t.Fatalf("int64 result = %d, contiguous %d, error %v", value, expected, got)
		}
	case 3:
		var value, expected uint64
		expected, want = baseline.DecodeUint64()
		value, got = d.DecodeUint64()
		if got == nil && value != expected || got != nil && value != 0 {
			t.Fatalf("uint64 result = %d, contiguous %d, error %v", value, expected, got)
		}
	case 4:
		var value, expected float32
		expected, want = baseline.DecodeFloat32()
		value, got = d.DecodeFloat32()
		if got == nil && math.Float32bits(value) != math.Float32bits(expected) || got != nil && math.Float32bits(value) != 0 {
			t.Fatalf("float32 result bits = %x, contiguous %x, error %v", math.Float32bits(value), math.Float32bits(expected), got)
		}
	case 5:
		var value, expected float64
		expected, want = baseline.DecodeFloat64()
		value, got = d.DecodeFloat64()
		if got == nil && math.IsNaN(value) && math.IsNaN(expected) && baseline.scratch[0] == 0xca {
			break // The contract leaves widened float32 NaN payload bits unspecified.
		}
		if got == nil && math.Float64bits(value) != math.Float64bits(expected) || got != nil && math.Float64bits(value) != 0 {
			t.Fatalf("float64 result bits = %x, contiguous %x, error %v", math.Float64bits(value), math.Float64bits(expected), got)
		}
	case 6:
		var value, expected string
		expected, want = baseline.DecodeString()
		value, got = d.DecodeString()
		if got == nil && value != expected || got != nil && value != "" {
			t.Fatalf("string result differs from contiguous input or is nonzero on failure: %v", got)
		}
	case 7:
		var value, expected []byte
		expected, want = baseline.DecodeBytes()
		value, got = d.DecodeBytes()
		if got == nil && (value == nil || !bytes.Equal(value, expected)) || got != nil && value != nil {
			t.Fatalf("binary result differs from contiguous input or is nonzero on failure: %v", got)
		}
	case 8:
		var value, expected uint32
		expected, want = baseline.DecodeArrayHeader()
		value, got = d.DecodeArrayHeader()
		if got == nil && value != expected || got != nil && value != 0 {
			t.Fatalf("array count = %d, contiguous %d, error %v", value, expected, got)
		}
	case 9:
		var value, expected uint32
		expected, want = baseline.DecodeMapHeader()
		value, got = d.DecodeMapHeader()
		if got == nil && value != expected || got != nil && value != 0 {
			t.Fatalf("map count = %d, contiguous %d, error %v", value, expected, got)
		}
	}
	return want, got
}
