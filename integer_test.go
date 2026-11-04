package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"testing"
)

func TestWriterInt64WireFixtures(t *testing.T) {
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
				output bytes.Buffer
				w      = NewWriter(&output)
				err    = w.EncodeInt64(tt.value)
			)
			if err != nil || !bytes.Equal(output.Bytes(), tt.want) {
				t.Fatalf("EncodeInt64(%d) = (%x, %v), want %x", tt.value, output.Bytes(), err, tt.want)
			}
		})
	}
}

func TestWriterUint64WireFixtures(t *testing.T) {
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
				output bytes.Buffer
				w      = NewWriter(&output)
				err    = w.EncodeUint64(tt.value)
			)
			if err != nil || !bytes.Equal(output.Bytes(), tt.want) {
				t.Fatalf("EncodeUint64(%d) = (%x, %v), want %x", tt.value, output.Bytes(), err, tt.want)
			}
		})
	}
}

func TestReadIntegers(t *testing.T) {
	type testcase struct {
		name        string
		src         []byte
		signed      int64
		unsigned    uint64
		signedErr   error
		unsignedErr error
	}
	tests := []testcase{
		{name: "zero", src: []byte{0x00}},
		{name: "one", src: []byte{0x01}, signed: 1, unsigned: 1},
		{name: "maximum positive fixint", src: []byte{0x7f}, signed: 127, unsigned: 127},
		{name: "above positive fixint", src: []byte{0xcc, 0x80}, signed: 128, unsigned: 128},
		{name: "maximum uint8", src: []byte{0xcc, 0xff}, signed: 255, unsigned: 255},
		{name: "above uint8", src: []byte{0xcd, 0x01, 0x00}, signed: 256, unsigned: 256},
		{name: "maximum uint16", src: []byte{0xcd, 0xff, 0xff}, signed: 65535, unsigned: 65535},
		{name: "above uint16", src: []byte{0xce, 0x00, 0x01, 0x00, 0x00}, signed: 65536, unsigned: 65536},
		{name: "maximum uint32", src: []byte{0xce, 0xff, 0xff, 0xff, 0xff}, signed: 4294967295, unsigned: 4294967295},
		{name: "above uint32", src: []byte{0xcf, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}, signed: 4294967296, unsigned: 4294967296},
		{name: "maximum int64 unsigned wire", src: []byte{0xcf, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, signed: math.MaxInt64, unsigned: math.MaxInt64},
		{name: "above int64", src: []byte{0xcf, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, unsigned: uint64(math.MaxInt64) + 1, signedErr: ErrOverflow},
		{name: "maximum uint64", src: []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, unsigned: math.MaxUint64, signedErr: ErrOverflow},
		{name: "negative one", src: []byte{0xff}, signed: -1, unsignedErr: ErrOverflow},
		{name: "minimum negative fixint", src: []byte{0xe0}, signed: -32, unsignedErr: ErrOverflow},
		{name: "below negative fixint", src: []byte{0xd0, 0xdf}, signed: -33, unsignedErr: ErrOverflow},
		{name: "minimum int8", src: []byte{0xd0, 0x80}, signed: -128, unsignedErr: ErrOverflow},
		{name: "below int8", src: []byte{0xd1, 0xff, 0x7f}, signed: -129, unsignedErr: ErrOverflow},
		{name: "minimum int16", src: []byte{0xd1, 0x80, 0x00}, signed: -32768, unsignedErr: ErrOverflow},
		{name: "below int16", src: []byte{0xd2, 0xff, 0xff, 0x7f, 0xff}, signed: -32769, unsignedErr: ErrOverflow},
		{name: "minimum int32", src: []byte{0xd2, 0x80, 0x00, 0x00, 0x00}, signed: -2147483648, unsignedErr: ErrOverflow},
		{name: "below int32", src: []byte{0xd3, 0xff, 0xff, 0xff, 0xff, 0x7f, 0xff, 0xff, 0xff}, signed: -2147483649, unsignedErr: ErrOverflow},
		{name: "minimum int64", src: []byte{0xd3, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, signed: math.MinInt64, unsignedErr: ErrOverflow},
		{name: "uint8 zero", src: []byte{0xcc, 0x00}},
		{name: "uint16 zero", src: []byte{0xcd, 0x00, 0x00}},
		{name: "uint32 zero", src: []byte{0xce, 0x00, 0x00, 0x00, 0x00}},
		{name: "uint64 zero", src: []byte{0xcf, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "uint8 one", src: []byte{0xcc, 0x01}, signed: 1, unsigned: 1},
		{name: "uint16 one", src: []byte{0xcd, 0x00, 0x01}, signed: 1, unsigned: 1},
		{name: "uint32 one", src: []byte{0xce, 0x00, 0x00, 0x00, 0x01}, signed: 1, unsigned: 1},
		{name: "uint64 one", src: []byte{0xcf, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, signed: 1, unsigned: 1},
		{name: "int8 zero", src: []byte{0xd0, 0x00}},
		{name: "int16 zero", src: []byte{0xd1, 0x00, 0x00}},
		{name: "int32 zero", src: []byte{0xd2, 0x00, 0x00, 0x00, 0x00}},
		{name: "int64 zero", src: []byte{0xd3, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "int8 one", src: []byte{0xd0, 0x01}, signed: 1, unsigned: 1},
		{name: "int16 one", src: []byte{0xd1, 0x00, 0x01}, signed: 1, unsigned: 1},
		{name: "int32 one", src: []byte{0xd2, 0x00, 0x00, 0x00, 0x01}, signed: 1, unsigned: 1},
		{name: "int64 one", src: []byte{0xd3, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}, signed: 1, unsigned: 1},
		{name: "maximum int8", src: []byte{0xd0, 0x7f}, signed: 127, unsigned: 127},
		{name: "maximum int16", src: []byte{0xd1, 0x7f, 0xff}, signed: 32767, unsigned: 32767},
		{name: "maximum int32", src: []byte{0xd2, 0x7f, 0xff, 0xff, 0xff}, signed: 2147483647, unsigned: 2147483647},
		{name: "maximum int64 signed wire", src: []byte{0xd3, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, signed: math.MaxInt64, unsigned: math.MaxInt64},
		{name: "int8 negative one", src: []byte{0xd0, 0xff}, signed: -1, unsignedErr: ErrOverflow},
		{name: "int16 negative one", src: []byte{0xd1, 0xff, 0xff}, signed: -1, unsignedErr: ErrOverflow},
		{name: "int32 negative one", src: []byte{0xd2, 0xff, 0xff, 0xff, 0xff}, signed: -1, unsignedErr: ErrOverflow},
		{name: "int64 negative one", src: []byte{0xd3, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, signed: -1, unsignedErr: ErrOverflow},
		{name: "nil input", signedErr: ErrTruncated, unsignedErr: ErrTruncated},
		{name: "empty input", src: []byte{}, signedErr: ErrTruncated, unsignedErr: ErrTruncated},
		{name: "reserved", src: []byte{0xc1}, signedErr: ErrInvalidFormat, unsignedErr: ErrInvalidFormat},
		{name: "ext8", src: []byte{0xc7}, signedErr: ErrUnsupportedType, unsignedErr: ErrUnsupportedType},
		{name: "ext16", src: []byte{0xc8}, signedErr: ErrUnsupportedType, unsignedErr: ErrUnsupportedType},
		{name: "ext32", src: []byte{0xc9}, signedErr: ErrUnsupportedType, unsignedErr: ErrUnsupportedType},
		{name: "fixext1", src: []byte{0xd4}, signedErr: ErrUnsupportedType, unsignedErr: ErrUnsupportedType},
		{name: "fixext2", src: []byte{0xd5}, signedErr: ErrUnsupportedType, unsignedErr: ErrUnsupportedType},
		{name: "fixext4", src: []byte{0xd6}, signedErr: ErrUnsupportedType, unsignedErr: ErrUnsupportedType},
		{name: "fixext8", src: []byte{0xd7}, signedErr: ErrUnsupportedType, unsignedErr: ErrUnsupportedType},
		{name: "fixext16", src: []byte{0xd8}, signedErr: ErrUnsupportedType, unsignedErr: ErrUnsupportedType},
		{name: "nil value", src: []byte{0xc0}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "false", src: []byte{0xc2}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "true", src: []byte{0xc3}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "incomplete float32", src: []byte{0xca}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "incomplete float64", src: []byte{0xcb}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "fixstr", src: []byte{0xa1}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "incomplete str32", src: []byte{0xdb}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "incomplete bin32", src: []byte{0xc6}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "fixarray", src: []byte{0x90}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "incomplete array16", src: []byte{0xdc}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "fixmap", src: []byte{0x80}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
		{name: "incomplete map32", src: []byte{0xdf}, signedErr: ErrTypeMismatch, unsignedErr: ErrTypeMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inputs := [][]byte{tt.src}
			if len(tt.src) > 0 {
				inputs = append(inputs, append(append([]byte(nil), tt.src...), 0xc3, 0xc1))
			}
			for index, src := range inputs {
				t.Run(fmt.Sprintf("input_%d", index), func(t *testing.T) {
					var (
						before        = append([]byte(nil), src...)
						signedCount   int
						unsignedCount int
						signed        int64
						unsigned      uint64
						signedErr     error
						unsignedErr   error
						wantSignedN   int
						wantUnsignedN int
					)
					if tt.signedErr == nil {
						wantSignedN = len(tt.src)
					}
					if tt.unsignedErr == nil {
						wantUnsignedN = len(tt.src)
					}
					signed, signedCount, signedErr = ReadInt64(src)
					if signed != tt.signed || signedCount != wantSignedN || !errors.Is(signedErr, tt.signedErr) {
						t.Fatalf("ReadInt64(%x) = (%d, %d, %v), want (%d, %d, %v)", src, signed, signedCount, signedErr, tt.signed, wantSignedN, tt.signedErr)
					}
					if !bytes.Equal(src, before) {
						t.Fatal("ReadInt64() modified source bytes")
					}
					unsigned, unsignedCount, unsignedErr = ReadUint64(src)
					if unsigned != tt.unsigned || unsignedCount != wantUnsignedN || !errors.Is(unsignedErr, tt.unsignedErr) {
						t.Fatalf("ReadUint64(%x) = (%d, %d, %v), want (%d, %d, %v)", src, unsigned, unsignedCount, unsignedErr, tt.unsigned, wantUnsignedN, tt.unsignedErr)
					}
					if !bytes.Equal(src, before) {
						t.Fatal("ReadUint64() modified source bytes")
					}
				})
			}
		})
	}
}

func TestReadIntegersTruncated(t *testing.T) {
	type testcase struct {
		name string
		src  []byte
	}
	tests := []testcase{
		{name: "uint8", src: []byte{0xcc, 0xff}},
		{name: "uint16", src: []byte{0xcd, 0xff, 0xff}},
		{name: "uint32", src: []byte{0xce, 0xff, 0xff, 0xff, 0xff}},
		{name: "uint64", src: []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "int8", src: []byte{0xd0, 0x80}},
		{name: "int16", src: []byte{0xd1, 0x80, 0x00}},
		{name: "int32", src: []byte{0xd2, 0x80, 0x00, 0x00, 0x00}},
		{name: "int64", src: []byte{0xd3, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for length := 0; length < len(tt.src); length++ {
				t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
					var (
						src           = tt.src[:length]
						before        = append([]byte(nil), tt.src...)
						signed        int64
						unsigned      uint64
						signedCount   int
						unsignedCount int
						signedErr     error
						unsignedErr   error
					)
					signed, signedCount, signedErr = ReadInt64(src)
					if signed != 0 || signedCount != 0 || !errors.Is(signedErr, ErrTruncated) {
						t.Fatalf("ReadInt64(%x) = (%d, %d, %v), want (0, 0, ErrTruncated)", src, signed, signedCount, signedErr)
					}
					unsigned, unsignedCount, unsignedErr = ReadUint64(src)
					if unsigned != 0 || unsignedCount != 0 || !errors.Is(unsignedErr, ErrTruncated) {
						t.Fatalf("ReadUint64(%x) = (%d, %d, %v), want (0, 0, ErrTruncated)", src, unsigned, unsignedCount, unsignedErr)
					}
					if !bytes.Equal(tt.src, before) {
						t.Fatal("integer reader modified source backing bytes")
					}
				})
			}
		})
	}
}
