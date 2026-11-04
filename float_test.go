package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"testing"
)

func TestFloat32Wire(t *testing.T) {
	type testcase struct {
		name string
		bits uint32
		wire []byte
	}
	tests := []testcase{
		{name: "positive zero", bits: 0x00000000, wire: []byte{0xca, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative zero", bits: 0x80000000, wire: []byte{0xca, 0x80, 0x00, 0x00, 0x00}},
		{name: "one and a half", bits: 0x3fc00000, wire: []byte{0xca, 0x3f, 0xc0, 0x00, 0x00}},
		{name: "negative two and a half", bits: 0xc0200000, wire: []byte{0xca, 0xc0, 0x20, 0x00, 0x00}},
		{name: "smallest subnormal", bits: 0x00000001, wire: []byte{0xca, 0x00, 0x00, 0x00, 0x01}},
		{name: "negative smallest subnormal", bits: 0x80000001, wire: []byte{0xca, 0x80, 0x00, 0x00, 0x01}},
		{name: "maximum finite", bits: 0x7f7fffff, wire: []byte{0xca, 0x7f, 0x7f, 0xff, 0xff}},
		{name: "minimum finite", bits: 0xff7fffff, wire: []byte{0xca, 0xff, 0x7f, 0xff, 0xff}},
		{name: "positive infinity", bits: 0x7f800000, wire: []byte{0xca, 0x7f, 0x80, 0x00, 0x00}},
		{name: "negative infinity", bits: 0xff800000, wire: []byte{0xca, 0xff, 0x80, 0x00, 0x00}},
		{name: "quiet NaN payload", bits: 0x7fc12345, wire: []byte{0xca, 0x7f, 0xc1, 0x23, 0x45}},
		{name: "negative quiet NaN payload", bits: 0xffc12345, wire: []byte{0xca, 0xff, 0xc1, 0x23, 0x45}},
		{name: "signaling NaN payload", bits: 0x7f812345, wire: []byte{0xca, 0x7f, 0x81, 0x23, 0x45}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				input     = math.Float32frombits(tt.bits)
				output    bytes.Buffer
				encodeErr = NewWriter(&output).EncodeFloat32(input)
			)
			if encodeErr != nil || !bytes.Equal(output.Bytes(), tt.wire) {
				t.Fatalf("EncodeFloat32(bits %x) = (%x, %v), want %x", tt.bits, output.Bytes(), encodeErr, tt.wire)
			}
			for _, trailing := range [][]byte{nil, {0xc3, 0xc1}} {
				var (
					src    = append(append([]byte(nil), tt.wire...), trailing...)
					before = append([]byte(nil), src...)
					value  float32
					wide   float64
					n      int
					err    error
				)
				value, n, err = ReadFloat32(src)
				if err != nil || n != len(tt.wire) || math.Float32bits(value) != tt.bits {
					t.Fatalf("ReadFloat32(%x) = (bits %08x, %d, %v)", src, math.Float32bits(value), n, err)
				}
				if !bytes.Equal(src, before) {
					t.Fatal("ReadFloat32() modified source bytes")
				}
				wide, n, err = ReadFloat64(src)
				if err != nil || n != len(tt.wire) {
					t.Fatalf("ReadFloat64(%x) = (%v, %d, %v)", src, wide, n, err)
				}
				if math.IsNaN(float64(input)) {
					if !math.IsNaN(wide) {
						t.Fatalf("ReadFloat64(%x) = %v, want NaN", src, wide)
					}
				} else if math.Float64bits(wide) != math.Float64bits(float64(input)) {
					t.Fatalf("ReadFloat64(%x) bits = %016x, want %016x", src, math.Float64bits(wide), math.Float64bits(float64(input)))
				}
				if !bytes.Equal(src, before) {
					t.Fatal("ReadFloat64() modified source bytes")
				}
			}
		})
	}
}

func TestFloat64Wire(t *testing.T) {
	type testcase struct {
		name string
		bits uint64
		wire []byte
	}
	tests := []testcase{
		{name: "positive zero", bits: 0x0000000000000000, wire: []byte{0xcb, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative zero", bits: 0x8000000000000000, wire: []byte{0xcb, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "one and a half", bits: 0x3ff8000000000000, wire: []byte{0xcb, 0x3f, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative two and a half", bits: 0xc004000000000000, wire: []byte{0xcb, 0xc0, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "smallest subnormal", bits: 0x0000000000000001, wire: []byte{0xcb, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}},
		{name: "negative smallest subnormal", bits: 0x8000000000000001, wire: []byte{0xcb, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}},
		{name: "maximum finite", bits: 0x7fefffffffffffff, wire: []byte{0xcb, 0x7f, 0xef, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "minimum finite", bits: 0xffefffffffffffff, wire: []byte{0xcb, 0xff, 0xef, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "positive infinity", bits: 0x7ff0000000000000, wire: []byte{0xcb, 0x7f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative infinity", bits: 0xfff0000000000000, wire: []byte{0xcb, 0xff, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "quiet NaN payload", bits: 0x7ff8123456789abc, wire: []byte{0xcb, 0x7f, 0xf8, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
		{name: "negative quiet NaN payload", bits: 0xfff8123456789abc, wire: []byte{0xcb, 0xff, 0xf8, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
		{name: "signaling NaN payload", bits: 0x7ff0123456789abc, wire: []byte{0xcb, 0x7f, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				input     = math.Float64frombits(tt.bits)
				output    bytes.Buffer
				encodeErr = NewWriter(&output).EncodeFloat64(input)
			)
			if encodeErr != nil || !bytes.Equal(output.Bytes(), tt.wire) {
				t.Fatalf("EncodeFloat64(bits %x) = (%x, %v), want %x", tt.bits, output.Bytes(), encodeErr, tt.wire)
			}
			for _, trailing := range [][]byte{nil, {0xc3, 0xc1}} {
				var (
					src    = append(append([]byte(nil), tt.wire...), trailing...)
					before = append([]byte(nil), src...)
					value  float64
					narrow float32
					n      int
					err    error
				)
				value, n, err = ReadFloat64(src)
				if err != nil || n != len(tt.wire) || math.Float64bits(value) != tt.bits {
					t.Fatalf("ReadFloat64(%x) = (bits %016x, %d, %v)", src, math.Float64bits(value), n, err)
				}
				if !bytes.Equal(src, before) {
					t.Fatal("ReadFloat64() modified source bytes")
				}
				narrow, n, err = ReadFloat32(src)
				if !errors.Is(err, ErrTypeMismatch) || n != 0 || math.Float32bits(narrow) != 0 {
					t.Fatalf("ReadFloat32(%x) = (%v, %d, %v), want (0, 0, ErrTypeMismatch)", src, narrow, n, err)
				}
				if !bytes.Equal(src, before) {
					t.Fatal("ReadFloat32() modified source bytes")
				}
			}
		})
	}
}

func TestReadFloatsErrors(t *testing.T) {
	type testcase struct {
		name string
		src  []byte
		err  error
	}
	tests := []testcase{
		{name: "nil input", err: ErrTruncated},
		{name: "empty input", src: []byte{}, err: ErrTruncated},
		{name: "reserved", src: []byte{0xc1}, err: ErrInvalidFormat},
		{name: "ext8", src: []byte{0xc7}, err: ErrUnsupportedType},
		{name: "ext16", src: []byte{0xc8}, err: ErrUnsupportedType},
		{name: "ext32", src: []byte{0xc9}, err: ErrUnsupportedType},
		{name: "fixext1", src: []byte{0xd4}, err: ErrUnsupportedType},
		{name: "fixext2", src: []byte{0xd5}, err: ErrUnsupportedType},
		{name: "fixext4", src: []byte{0xd6}, err: ErrUnsupportedType},
		{name: "fixext8", src: []byte{0xd7}, err: ErrUnsupportedType},
		{name: "fixext16", src: []byte{0xd8}, err: ErrUnsupportedType},
		{name: "nil value", src: []byte{0xc0}, err: ErrTypeMismatch},
		{name: "false", src: []byte{0xc2}, err: ErrTypeMismatch},
		{name: "true", src: []byte{0xc3}, err: ErrTypeMismatch},
		{name: "positive fixint", src: []byte{0x01}, err: ErrTypeMismatch},
		{name: "negative fixint", src: []byte{0xff}, err: ErrTypeMismatch},
		{name: "incomplete unsigned integer", src: []byte{0xcf}, err: ErrTypeMismatch},
		{name: "incomplete signed integer", src: []byte{0xd3}, err: ErrTypeMismatch},
		{name: "complete integer", src: []byte{0xcc, 0x01}, err: ErrTypeMismatch},
		{name: "fixstr", src: []byte{0xa1}, err: ErrTypeMismatch},
		{name: "incomplete string", src: []byte{0xdb}, err: ErrTypeMismatch},
		{name: "incomplete binary", src: []byte{0xc6}, err: ErrTypeMismatch},
		{name: "fixarray", src: []byte{0x90}, err: ErrTypeMismatch},
		{name: "incomplete array", src: []byte{0xdd}, err: ErrTypeMismatch},
		{name: "fixmap", src: []byte{0x80}, err: ErrTypeMismatch},
		{name: "incomplete map", src: []byte{0xdf}, err: ErrTypeMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before = append([]byte(nil), tt.src...)
				narrow float32
				wide   float64
				n      int
				err    error
			)
			narrow, n, err = ReadFloat32(tt.src)
			if !errors.Is(err, tt.err) || n != 0 || math.Float32bits(narrow) != 0 {
				t.Fatalf("ReadFloat32(%x) = (%v, %d, %v), want (0, 0, %v)", tt.src, narrow, n, err, tt.err)
			}
			if !bytes.Equal(tt.src, before) {
				t.Fatal("ReadFloat32() modified source bytes")
			}
			wide, n, err = ReadFloat64(tt.src)
			if !errors.Is(err, tt.err) || n != 0 || math.Float64bits(wide) != 0 {
				t.Fatalf("ReadFloat64(%x) = (%v, %d, %v), want (0, 0, %v)", tt.src, wide, n, err, tt.err)
			}
			if !bytes.Equal(tt.src, before) {
				t.Fatal("ReadFloat64() modified source bytes")
			}
		})
	}
}

func TestReadFloatsTruncated(t *testing.T) {
	type testcase struct {
		name string
		wire []byte
	}
	tests := []testcase{
		{name: "float32", wire: []byte{0xca, 0x3f, 0xc0, 0x00, 0x00}},
		{name: "float64", wire: []byte{0xcb, 0x3f, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for length := 0; length < len(tt.wire); length++ {
				t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
					var (
						src       = tt.wire[:length]
						before    = append([]byte(nil), tt.wire...)
						narrowErr = ErrTruncated
						narrow    float32
						wide      float64
						n         int
						err       error
					)
					if length > 0 && tt.wire[0] == 0xcb {
						narrowErr = ErrTypeMismatch
					}
					narrow, n, err = ReadFloat32(src)
					if !errors.Is(err, narrowErr) || n != 0 || math.Float32bits(narrow) != 0 {
						t.Fatalf("ReadFloat32(%x) = (%v, %d, %v), want (0, 0, %v)", src, narrow, n, err, narrowErr)
					}
					wide, n, err = ReadFloat64(src)
					if !errors.Is(err, ErrTruncated) || n != 0 || math.Float64bits(wide) != 0 {
						t.Fatalf("ReadFloat64(%x) = (%v, %d, %v), want (0, 0, ErrTruncated)", src, wide, n, err)
					}
					if !bytes.Equal(tt.wire, before) {
						t.Fatal("float reader modified source backing bytes")
					}
				})
			}
		})
	}
}
