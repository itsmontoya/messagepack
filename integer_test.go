package messagepack

import (
	"bytes"
	"math"
	"testing"
)

func TestAppendInt64(t *testing.T) {
	type testcase struct {
		name  string
		value int64
		want  []byte
	}
	tests := []testcase{
		{name: "minimum int64", value: math.MinInt64, want: []byte{0xd3, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "below int32", value: -2147483649, want: []byte{0xd3, 0xff, 0xff, 0xff, 0xff, 0x7f, 0xff, 0xff, 0xff}},
		{name: "minimum int32", value: -2147483648, want: []byte{0xd2, 0x80, 0x00, 0x00, 0x00}},
		{name: "below int16", value: -32769, want: []byte{0xd2, 0xff, 0xff, 0x7f, 0xff}},
		{name: "minimum int16", value: -32768, want: []byte{0xd1, 0x80, 0x00}},
		{name: "below int8", value: -129, want: []byte{0xd1, 0xff, 0x7f}},
		{name: "minimum int8", value: -128, want: []byte{0xd0, 0x80}},
		{name: "below negative fixint", value: -33, want: []byte{0xd0, 0xdf}},
		{name: "minimum negative fixint", value: -32, want: []byte{0xe0}},
		{name: "negative one", value: -1, want: []byte{0xff}},
		{name: "zero", value: 0, want: []byte{0x00}},
		{name: "one", value: 1, want: []byte{0x01}},
		{name: "maximum positive fixint", value: 127, want: []byte{0x7f}},
		{name: "above positive fixint", value: 128, want: []byte{0xcc, 0x80}},
		{name: "maximum uint8", value: 255, want: []byte{0xcc, 0xff}},
		{name: "above uint8", value: 256, want: []byte{0xcd, 0x01, 0x00}},
		{name: "maximum uint16", value: 65535, want: []byte{0xcd, 0xff, 0xff}},
		{name: "above uint16", value: 65536, want: []byte{0xce, 0x00, 0x01, 0x00, 0x00}},
		{name: "maximum uint32", value: 4294967295, want: []byte{0xce, 0xff, 0xff, 0xff, 0xff}},
		{name: "above uint32", value: 4294967296, want: []byte{0xcf, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}},
		{name: "maximum int64", value: math.MaxInt64, want: []byte{0xcf, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				reused    = make([]byte, 2, len(tt.want)+2)
				scenarios = []struct {
					name  string
					dst   []byte
					reuse bool
				}{
					{name: "fresh"},
					{name: "prefix with growth", dst: []byte{0xaa, 0xbb}},
					{name: "prefix with capacity", dst: reused, reuse: true},
					{name: "reset buffer", dst: make([]byte, 0, len(tt.want)), reuse: true},
				}
			)
			reused[0], reused[1] = 0xaa, 0xbb
			for _, scenario := range scenarios {
				t.Run(scenario.name, func(t *testing.T) {
					var (
						want = append(append([]byte(nil), scenario.dst...), tt.want...)
						out  = AppendInt64(scenario.dst, tt.value)
					)
					if !bytes.Equal(out, want) {
						t.Fatalf("AppendInt64(%d) = %x, want %x", tt.value, out, want)
					}
					if scenario.reuse && &out[0] != &scenario.dst[:cap(scenario.dst)][0] {
						t.Fatal("AppendInt64() did not reuse available destination capacity")
					}
				})
			}
			if tt.value >= 0 && !bytes.Equal(AppendInt64(nil, tt.value), AppendUint64(nil, uint64(tt.value))) {
				t.Fatal("nonnegative signed and unsigned encodings differ")
			}
		})
	}
}

func TestAppendUint64(t *testing.T) {
	type testcase struct {
		name  string
		value uint64
		want  []byte
	}
	tests := []testcase{
		{name: "zero", value: 0, want: []byte{0x00}},
		{name: "one", value: 1, want: []byte{0x01}},
		{name: "maximum positive fixint", value: 127, want: []byte{0x7f}},
		{name: "above positive fixint", value: 128, want: []byte{0xcc, 0x80}},
		{name: "maximum uint8", value: 255, want: []byte{0xcc, 0xff}},
		{name: "above uint8", value: 256, want: []byte{0xcd, 0x01, 0x00}},
		{name: "maximum uint16", value: 65535, want: []byte{0xcd, 0xff, 0xff}},
		{name: "above uint16", value: 65536, want: []byte{0xce, 0x00, 0x01, 0x00, 0x00}},
		{name: "maximum uint32", value: 4294967295, want: []byte{0xce, 0xff, 0xff, 0xff, 0xff}},
		{name: "above uint32", value: 4294967296, want: []byte{0xcf, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}},
		{name: "maximum int64", value: math.MaxInt64, want: []byte{0xcf, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "above int64", value: uint64(math.MaxInt64) + 1, want: []byte{0xcf, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "maximum uint64", value: math.MaxUint64, want: []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				reused    = make([]byte, 2, len(tt.want)+2)
				scenarios = []struct {
					name  string
					dst   []byte
					reuse bool
				}{
					{name: "fresh"},
					{name: "prefix with growth", dst: []byte{0xaa, 0xbb}},
					{name: "prefix with capacity", dst: reused, reuse: true},
					{name: "reset buffer", dst: make([]byte, 0, len(tt.want)), reuse: true},
				}
			)
			reused[0], reused[1] = 0xaa, 0xbb
			for _, scenario := range scenarios {
				t.Run(scenario.name, func(t *testing.T) {
					var (
						want = append(append([]byte(nil), scenario.dst...), tt.want...)
						out  = AppendUint64(scenario.dst, tt.value)
					)
					if !bytes.Equal(out, want) {
						t.Fatalf("AppendUint64(%d) = %x, want %x", tt.value, out, want)
					}
					if scenario.reuse && &out[0] != &scenario.dst[:cap(scenario.dst)][0] {
						t.Fatal("AppendUint64() did not reuse available destination capacity")
					}
				})
			}
		})
	}
}
