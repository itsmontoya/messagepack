package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"testing"
)

func TestReaderNilAndBool(t *testing.T) {
	tests := []struct {
		name string
		wire []byte
		want bool
	}{
		{name: "nil", wire: []byte{0xc0}},
		{name: "false", wire: []byte{0xc2}},
		{name: "true", wire: []byte{0xc3}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r   = &decoderTestReader{data: tt.wire, endErr: io.EOF}
				d   = NewReader(r)
				err error
			)

			if tt.name == "nil" {
				err = d.DecodeNil()
			} else {
				var value bool
				value, err = d.DecodeBool()
				if value != tt.want {
					t.Fatalf("DecodeBool() = %v, want %v", value, tt.want)
				}
			}

			if err != nil || r.reads != 1 || r.position != 1 {
				t.Fatalf("decode = %v, reads %d, consumed %d", err, r.reads, r.position)
			}

			err = d.DecodeNil()
			if !errors.Is(err, io.EOF) || errors.Is(err, errFuzzTruncated) || r.reads != 1 {
				t.Fatalf("pending EOF = %v after %d reads", err, r.reads)
			}

			requireReaderScalarFailure(t, d, err)
			if r.reads != 1 {
				t.Fatal("terminal EOF caused additional reads")
			}
		})
	}
}

func TestReaderIntegerWire(t *testing.T) {
	tests := []struct {
		name        string
		wire        []byte
		signed      int64
		unsigned    uint64
		signedErr   error
		unsignedErr error
	}{
		{name: "zero", wire: []byte{0x00}},
		{name: "positive fixint maximum", wire: []byte{0x7f}, signed: 127, unsigned: 127},
		{name: "uint8 minimum", wire: []byte{0xcc, 0x80}, signed: 128, unsigned: 128},
		{name: "uint8 maximum", wire: []byte{0xcc, 0xff}, signed: 255, unsigned: 255},
		{name: "uint16 minimum", wire: []byte{0xcd, 0x01, 0x00}, signed: 256, unsigned: 256},
		{name: "uint16 maximum", wire: []byte{0xcd, 0xff, 0xff}, signed: 65535, unsigned: 65535},
		{name: "uint32 minimum", wire: []byte{0xce, 0x00, 0x01, 0x00, 0x00}, signed: 65536, unsigned: 65536},
		{name: "uint32 maximum", wire: []byte{0xce, 0xff, 0xff, 0xff, 0xff}, signed: 4294967295, unsigned: 4294967295},
		{name: "uint64 minimum", wire: []byte{0xcf, 0, 0, 0, 1, 0, 0, 0, 0}, signed: 4294967296, unsigned: 4294967296},
		{name: "maximum int64 unsigned wire", wire: []byte{0xcf, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, signed: math.MaxInt64, unsigned: math.MaxInt64},
		{name: "above int64", wire: []byte{0xcf, 0x80, 0, 0, 0, 0, 0, 0, 0}, unsigned: uint64(math.MaxInt64) + 1, signedErr: ErrOverflow},
		{name: "maximum uint64", wire: []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, unsigned: math.MaxUint64, signedErr: ErrOverflow},
		{name: "negative fixint maximum", wire: []byte{0xff}, signed: -1, unsignedErr: ErrOverflow},
		{name: "negative fixint minimum", wire: []byte{0xe0}, signed: -32, unsignedErr: ErrOverflow},
		{name: "int8 maximum negative", wire: []byte{0xd0, 0xdf}, signed: -33, unsignedErr: ErrOverflow},
		{name: "int8 minimum", wire: []byte{0xd0, 0x80}, signed: -128, unsignedErr: ErrOverflow},
		{name: "int16 maximum negative", wire: []byte{0xd1, 0xff, 0x7f}, signed: -129, unsignedErr: ErrOverflow},
		{name: "int16 minimum", wire: []byte{0xd1, 0x80, 0x00}, signed: -32768, unsignedErr: ErrOverflow},
		{name: "int32 maximum negative", wire: []byte{0xd2, 0xff, 0xff, 0x7f, 0xff}, signed: -32769, unsignedErr: ErrOverflow},
		{name: "int32 minimum", wire: []byte{0xd2, 0x80, 0, 0, 0}, signed: -2147483648, unsignedErr: ErrOverflow},
		{name: "int64 maximum negative", wire: []byte{0xd3, 0xff, 0xff, 0xff, 0xff, 0x7f, 0xff, 0xff, 0xff}, signed: -2147483649, unsignedErr: ErrOverflow},
		{name: "int64 minimum", wire: []byte{0xd3, 0x80, 0, 0, 0, 0, 0, 0, 0}, signed: math.MinInt64, unsignedErr: ErrOverflow},
		{name: "positive int8", wire: []byte{0xd0, 0x7f}, signed: 127, unsigned: 127},
		{name: "positive int16", wire: []byte{0xd1, 0x7f, 0xff}, signed: 32767, unsigned: 32767},
		{name: "positive int32", wire: []byte{0xd2, 0x7f, 0xff, 0xff, 0xff}, signed: 2147483647, unsigned: 2147483647},
		{name: "positive int64", wire: []byte{0xd3, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, signed: math.MaxInt64, unsigned: math.MaxInt64},
		{name: "wide uint64 one", wire: []byte{0xcf, 0, 0, 0, 0, 0, 0, 0, 1}, signed: 1, unsigned: 1},
		{name: "wide int64 one", wire: []byte{0xd3, 0, 0, 0, 0, 0, 0, 0, 1}, signed: 1, unsigned: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				src = append(append([]byte(nil), tt.wire...), 0xc3)
				r   = &decoderTestReader{data: src, maxRead: 1}
				d   = NewReader(r)
			)

			signed, signedErr := d.DecodeInt64()
			if signed != tt.signed || signedErr != tt.signedErr || r.position != len(tt.wire) {
				t.Fatalf("DecodeInt64(%x) = (%d, %v), consumed %d", tt.wire, signed, signedErr, r.position)
			}

			if signedErr != nil {
				requireReaderScalarFailure(t, d, signedErr)
			} else {
				value, nextErr := d.DecodeBool()
				if !value || nextErr != nil {
					t.Fatalf("following bool = (%v, %v)", value, nextErr)
				}
			}

			r = &decoderTestReader{data: src, maxRead: 1}
			d.Reset(r)
			unsigned, unsignedErr := d.DecodeUint64()
			if unsigned != tt.unsigned || unsignedErr != tt.unsignedErr || r.position != len(tt.wire) {
				t.Fatalf("DecodeUint64(%x) = (%d, %v), consumed %d", tt.wire, unsigned, unsignedErr, r.position)
			}

			if unsignedErr != nil {
				requireReaderScalarFailure(t, d, unsignedErr)
			} else {
				value, nextErr := d.DecodeBool()
				if !value || nextErr != nil {
					t.Fatalf("following bool = (%v, %v)", value, nextErr)
				}
			}

		})
	}
}

func TestReaderFloat32Wire(t *testing.T) {
	tests := []struct {
		name string
		bits uint32
		wire []byte
	}{
		{name: "positive zero", bits: 0, wire: []byte{0xca, 0, 0, 0, 0}},
		{name: "negative zero", bits: 0x80000000, wire: []byte{0xca, 0x80, 0, 0, 0}},
		{name: "one and a half", bits: 0x3fc00000, wire: []byte{0xca, 0x3f, 0xc0, 0, 0}},
		{name: "smallest subnormal", bits: 1, wire: []byte{0xca, 0, 0, 0, 1}},
		{name: "maximum finite", bits: 0x7f7fffff, wire: []byte{0xca, 0x7f, 0x7f, 0xff, 0xff}},
		{name: "positive infinity", bits: 0x7f800000, wire: []byte{0xca, 0x7f, 0x80, 0, 0}},
		{name: "negative infinity", bits: 0xff800000, wire: []byte{0xca, 0xff, 0x80, 0, 0}},
		{name: "quiet NaN", bits: 0x7fc12345, wire: []byte{0xca, 0x7f, 0xc1, 0x23, 0x45}},
		{name: "signaling NaN", bits: 0x7f812345, wire: []byte{0xca, 0x7f, 0x81, 0x23, 0x45}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r = &decoderTestReader{data: tt.wire, maxRead: 1, endErr: io.EOF}
				d = NewReader(r)
			)

			value, err := d.DecodeFloat32()
			if err != nil || math.Float32bits(value) != tt.bits {
				t.Fatalf("DecodeFloat32(%x) = (%08x, %v)", tt.wire, math.Float32bits(value), err)
			}

			d.Reset(bytes.NewReader(tt.wire))
			wide, wideErr := d.DecodeFloat64()
			want := float64(math.Float32frombits(tt.bits))
			if wideErr != nil {
				t.Fatal(wideErr)
			}

			if math.IsNaN(want) {
				if !math.IsNaN(wide) {
					t.Fatalf("widened NaN = %v", wide)
				}
			} else if math.Float64bits(wide) != math.Float64bits(want) {
				t.Fatalf("widened bits = %016x, want %016x", math.Float64bits(wide), math.Float64bits(want))
			}
		})
	}
}

func TestReaderFloat64Wire(t *testing.T) {
	tests := []struct {
		name string
		bits uint64
		wire []byte
	}{
		{name: "positive zero", bits: 0, wire: []byte{0xcb, 0, 0, 0, 0, 0, 0, 0, 0}},
		{name: "negative zero", bits: 0x8000000000000000, wire: []byte{0xcb, 0x80, 0, 0, 0, 0, 0, 0, 0}},
		{name: "one and a half", bits: 0x3ff8000000000000, wire: []byte{0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}},
		{name: "smallest subnormal", bits: 1, wire: []byte{0xcb, 0, 0, 0, 0, 0, 0, 0, 1}},
		{name: "maximum finite", bits: 0x7fefffffffffffff, wire: []byte{0xcb, 0x7f, 0xef, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "positive infinity", bits: 0x7ff0000000000000, wire: []byte{0xcb, 0x7f, 0xf0, 0, 0, 0, 0, 0, 0}},
		{name: "negative infinity", bits: 0xfff0000000000000, wire: []byte{0xcb, 0xff, 0xf0, 0, 0, 0, 0, 0, 0}},
		{name: "quiet NaN", bits: 0x7ff8123456789abc, wire: []byte{0xcb, 0x7f, 0xf8, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
		{name: "signaling NaN", bits: 0x7ff0123456789abc, wire: []byte{0xcb, 0x7f, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r = &decoderTestReader{data: tt.wire, maxRead: 1, endErr: io.EOF}
				d = NewReader(r)
			)

			value, err := d.DecodeFloat64()
			if err != nil || math.Float64bits(value) != tt.bits {
				t.Fatalf("DecodeFloat64(%x) = (%016x, %v)", tt.wire, math.Float64bits(value), err)
			}

			d.Reset(bytes.NewReader(tt.wire))
			narrow, narrowErr := d.DecodeFloat32()
			if narrowErr != ErrTypeMismatch || math.Float32bits(narrow) != 0 {
				t.Fatalf("narrowed float64 = (%v, %v)", narrow, narrowErr)
			}
		})
	}
}

func requireReaderScalarFailure(t *testing.T, d *Reader, want error) {
	t.Helper()
	if err := d.DecodeNil(); err != want {
		t.Fatalf("DecodeNil() terminal error = %v, want same %v", err, want)
	}

	valueBool, boolErr := d.DecodeBool()
	if valueBool || boolErr != want {
		t.Fatalf("DecodeBool() after failure = (%v, %v)", valueBool, boolErr)
	}

	valueInt, intErr := d.DecodeInt64()
	if valueInt != 0 || intErr != want {
		t.Fatalf("DecodeInt64() after failure = (%v, %v)", valueInt, intErr)
	}

	valueUint, uintErr := d.DecodeUint64()
	if valueUint != 0 || uintErr != want {
		t.Fatalf("DecodeUint64() after failure = (%v, %v)", valueUint, uintErr)
	}

	value32, float32Err := d.DecodeFloat32()
	if math.Float32bits(value32) != 0 || float32Err != want {
		t.Fatalf("DecodeFloat32() after failure = (%v, %v)", value32, float32Err)
	}

	value64, float64Err := d.DecodeFloat64()
	if math.Float64bits(value64) != 0 || float64Err != want {
		t.Fatalf("DecodeFloat64() after failure = (%v, %v)", value64, float64Err)
	}
}

func TestReaderScalarTruncation(t *testing.T) {
	var (
		integers = [][]byte{
			{0x01}, {0xff}, {0xcc, 0x80}, {0xcd, 0x01, 0x00},
			{0xce, 0, 0x01, 0, 0}, {0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			{0xd0, 0xff}, {0xd1, 0xff, 0xff}, {0xd2, 0xff, 0xff, 0xff, 0xff},
			{0xd3, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		}
		tests = []struct {
			name   string
			wires  [][]byte
			decode func(test *testing.T, d *Reader) (err error)
		}{
			{name: "nil", wires: [][]byte{{0xc0}}, decode: func(test *testing.T, d *Reader) (err error) {
				return d.DecodeNil()
			}},
			{name: "bool", wires: [][]byte{{0xc2}, {0xc3}}, decode: func(test *testing.T, d *Reader) (err error) {
				var value bool
				value, err = d.DecodeBool()
				if value {
					test.Fatal("failed boolean result is not zero")
				}

				return err
			}},
			{name: "int64", wires: integers, decode: func(test *testing.T, d *Reader) (err error) {
				var value int64
				value, err = d.DecodeInt64()
				if value != 0 {
					test.Fatal("failed signed result is not zero")
				}

				return err
			}},
			{name: "uint64", wires: integers, decode: func(test *testing.T, d *Reader) (err error) {
				var value uint64
				value, err = d.DecodeUint64()
				if value != 0 {
					test.Fatal("failed unsigned result is not zero")
				}

				return err
			}},
			{name: "float32", wires: [][]byte{{0xca, 0x3f, 0xc0, 0, 0}}, decode: func(test *testing.T, d *Reader) (err error) {
				var value float32
				value, err = d.DecodeFloat32()
				if math.Float32bits(value) != 0 {
					test.Fatal("failed float32 bits are not zero")
				}

				return err
			}},
			{name: "float64", wires: [][]byte{{0xca, 0x3f, 0xc0, 0, 0}, {0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}}, decode: func(test *testing.T, d *Reader) (err error) {
				var value float64
				value, err = d.DecodeFloat64()
				if math.Float64bits(value) != 0 {
					test.Fatal("failed float64 bits are not zero")
				}

				return err
			}},
		}
	)

	for _, tt := range tests {
		for _, wire := range tt.wires {
			for length := 0; length < len(wire); length++ {
				t.Run(fmt.Sprintf("%s/%x/length_%d", tt.name, wire[0], length), func(t *testing.T) {
					var (
						r    = &decoderTestReader{data: wire[:length], maxRead: 1}
						d    = NewReader(r)
						want = io.ErrUnexpectedEOF
						err  = tt.decode(t, d)
					)

					if length == 0 {
						want = io.EOF
					}

					if !errors.Is(err, want) || errors.Is(err, errFuzzTruncated) || err == want || r.position != length {
						t.Fatalf("truncation = %v, consumed %d, want wrapped %v after %d", err, r.position, want, length)
					}

					if length > 0 && errors.Is(err, io.EOF) {
						t.Fatal("incomplete value also matched clean EOF")
					}

					reads := r.reads
					requireReaderScalarFailure(t, d, err)
					if r.reads != reads {
						t.Fatal("failed scalar performed more I/O")
					}
				})
			}
		}
	}
}

func TestReaderReadErrorsAndEOF(t *testing.T) {
	var (
		failure = errors.New("test source failure")
		tests   = []struct {
			name string
			data []byte
			end  error
			want error
		}{
			{name: "no byte error", end: failure, want: failure},
			{name: "tag and error", data: []byte{0xcf}, end: failure, want: failure},
			{name: "partial body and error", data: []byte{0xcf, 0, 0, 0}, end: failure, want: failure},
			{name: "full body and error", data: []byte{0xcf, 0, 0, 0, 0, 0, 0, 0, 7}, end: failure, want: failure},
			{name: "tag and EOF", data: []byte{0xcf}, end: io.EOF, want: io.ErrUnexpectedEOF},
			{name: "partial body and EOF", data: []byte{0xcf, 0, 0, 0}, end: io.EOF, want: io.ErrUnexpectedEOF},
			{name: "full body and EOF", data: []byte{0xcf, 0, 0, 0, 0, 0, 0, 0, 7}, end: io.EOF},
		}
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r     = &decoderTestReader{data: tt.data, endErr: tt.end}
				d     = NewReader(r)
				value uint64
				err   error
			)

			value, err = d.DecodeUint64()
			if !errors.Is(err, tt.want) {
				t.Fatalf("DecodeUint64() = (%d, %v), want %v", value, err, tt.want)
			}

			if err == nil {
				if value != 7 {
					t.Fatalf("complete data with EOF = %d, want 7", value)
				}

				reads := r.reads
				err = d.DecodeNil()
				if !errors.Is(err, io.EOF) || r.reads != reads {
					t.Fatalf("pending EOF = %v after %d reads, want %d", err, r.reads, reads)
				}
			} else if value != 0 || err == tt.want || !strings.Contains(err.Error(), "decode uint64 source") {
				t.Fatalf("failure did not return zero/context: (%d, %v)", value, err)
			}

			reads := r.reads
			requireReaderScalarFailure(t, d, err)
			if r.reads != reads {
				t.Fatal("transport failure performed further reads")
			}
		})
	}
}

func TestReaderZeroProgress(t *testing.T) {
	tests := []struct {
		name         string
		initialEmpty int
		emptyBetween int
		want         error
		reads        int
	}{
		{name: "temporary empty reads", initialEmpty: 99, emptyBetween: 99, reads: 900},
		{name: "empty threshold before tag", initialEmpty: 100, want: io.ErrNoProgress, reads: 100},
		{name: "empty threshold after tag", emptyBetween: 100, want: io.ErrNoProgress, reads: 101},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r = &decoderTestReader{data: []byte{0xcf, 0, 0, 0, 0, 0, 0, 0, 7}, maxRead: 1,
					initialEmpty: tt.initialEmpty, emptyBetween: tt.emptyBetween}
				d = NewReader(r)
			)

			value, err := d.DecodeUint64()
			if !errors.Is(err, tt.want) || r.reads != tt.reads {
				t.Fatalf("DecodeUint64() = (%d, %v), reads %d, want error %v and %d reads", value, err, r.reads, tt.want, tt.reads)
			}

			if err == nil {
				if value != 7 {
					t.Fatalf("temporary empty reads changed value to %d", value)
				}

				return
			}

			if value != 0 || err == tt.want {
				t.Fatalf("zero progress did not return zero and wrapped error: (%d, %v)", value, err)
			}

			requireReaderScalarFailure(t, d, err)
			if r.reads != tt.reads {
				t.Fatal("zero-progress failure performed further reads")
			}
		})
	}
}

func TestReaderClassification(t *testing.T) {
	var (
		failure = errors.New("tag transport failure")
		tests   = []struct {
			name string
			tag  byte
			end  error
			want error
		}{
			{name: "reserved", tag: 0xc1, want: ErrInvalidFormat},
			{name: "ext8", tag: 0xc7, want: ErrUnsupportedType},
			{name: "ext16", tag: 0xc8, want: ErrUnsupportedType},
			{name: "ext32", tag: 0xc9, want: ErrUnsupportedType},
			{name: "fixext1", tag: 0xd4, want: ErrUnsupportedType},
			{name: "fixext2", tag: 0xd5, want: ErrUnsupportedType},
			{name: "fixext4", tag: 0xd6, want: ErrUnsupportedType},
			{name: "fixext8", tag: 0xd7, want: ErrUnsupportedType},
			{name: "fixext16", tag: 0xd8, want: ErrUnsupportedType},
			{name: "nil mismatch", tag: 0xc0, want: ErrTypeMismatch},
			{name: "fixint mismatch", tag: 0x01, want: ErrTypeMismatch},
			{name: "negative fixint mismatch", tag: 0xff, want: ErrTypeMismatch},
			{name: "incomplete integer mismatch", tag: 0xcd, want: ErrTypeMismatch},
			{name: "incomplete float mismatch", tag: 0xcb, want: ErrTypeMismatch},
			{name: "incomplete string mismatch", tag: 0xdb, want: ErrTypeMismatch},
			{name: "incomplete binary mismatch", tag: 0xc6, want: ErrTypeMismatch},
			{name: "incomplete array mismatch", tag: 0xdd, want: ErrTypeMismatch},
			{name: "incomplete map mismatch", tag: 0xdf, want: ErrTypeMismatch},
			{name: "reserved plus EOF", tag: 0xc1, end: io.EOF, want: ErrInvalidFormat},
			{name: "nonEOF error overrides reserved", tag: 0xc1, end: failure, want: failure},
			{name: "nonEOF error overrides valid tag", tag: 0xc3, end: failure, want: failure},
		}
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r = &decoderTestReader{data: []byte{tt.tag}, endErr: tt.end}
				d = NewReader(r)
			)

			value, err := d.DecodeBool()
			if value || !errors.Is(err, tt.want) || r.position != 1 || r.reads != 1 {
				t.Fatalf("DecodeBool(%x) = (%v, %v), consumed %d in %d reads", tt.tag, value, err, r.position, r.reads)
			}

			if tt.want != failure && err != tt.want {
				t.Fatalf("codec sentinel was wrapped: %v", err)
			}

			requireReaderScalarFailure(t, d, err)
			if r.reads != 1 {
				t.Fatal("classification failure read past its tag")
			}
		})
	}
}

func TestReaderConsecutiveScalars(t *testing.T) {
	var (
		wire = []byte{0xc0, 0xc3, 0xd0, 0xdf, 0xcd, 0x01, 0x00,
			0xca, 0x3f, 0xc0, 0, 0, 0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0, 0xc2}
		r = &decoderTestReader{data: wire}
		d = NewReader(r)
	)

	if err := d.DecodeNil(); err != nil || r.position != 1 {
		t.Fatalf("nil = %v at %d", err, r.position)
	}

	var valueBool, boolErr = d.DecodeBool()
	if !valueBool || boolErr != nil || r.position != 2 {
		t.Fatalf("bool = (%v, %v) at %d", valueBool, boolErr, r.position)
	}

	valueInt, intErr := d.DecodeInt64()
	if valueInt != -33 || intErr != nil || r.position != 4 {
		t.Fatalf("int64 = (%v, %v) at %d", valueInt, intErr, r.position)
	}

	valueUint, uintErr := d.DecodeUint64()
	if valueUint != 256 || uintErr != nil || r.position != 7 {
		t.Fatalf("uint64 = (%v, %v) at %d", valueUint, uintErr, r.position)
	}

	value32, err32 := d.DecodeFloat32()
	if value32 != 1.5 || err32 != nil || r.position != 12 {
		t.Fatalf("float32 = (%v, %v) at %d", value32, err32, r.position)
	}

	value64, err64 := d.DecodeFloat64()
	if value64 != 1.5 || err64 != nil || r.position != 21 {
		t.Fatalf("float64 = (%v, %v) at %d", value64, err64, r.position)
	}

	if !bytes.Equal(r.data[r.position:], []byte{0xc2}) {
		t.Fatal("following value was read ahead")
	}

	wantRequests := []int{1, 1, 1, 1, 1, 2, 1, 4, 1, 8}
	if fmt.Sprint(r.requests) != fmt.Sprint(wantRequests) {
		t.Fatalf("read requests = %v, want %v", r.requests, wantRequests)
	}

	valueBool, boolErr = d.DecodeBool()
	if valueBool || boolErr != nil {
		t.Fatalf("last bool = (%v, %v)", valueBool, boolErr)
	}

	if err := d.DecodeNil(); !errors.Is(err, io.EOF) {
		t.Fatalf("EOF after concatenated values = %v", err)
	}
}

func TestReaderNilFixtures(t *testing.T) {
	type testcase struct {
		name string
		src  []byte
		n    int
		err  error
	}
	tests := []testcase{
		{name: "nil", src: []byte{0xc0}, n: 1},
		{name: "trailing value", src: []byte{0xc0, 0xc3}, n: 1},
		{name: "trailing invalid tag", src: []byte{0xc0, 0xc1}, n: 1},
		{name: "nil input", err: errFuzzTruncated},
		{name: "empty input", src: []byte{}, err: errFuzzTruncated},
		{name: "reserved", src: []byte{0xc1}, err: ErrInvalidFormat},
		{name: "ext8", src: []byte{0xc7}, err: ErrUnsupportedType},
		{name: "ext16", src: []byte{0xc8}, err: ErrUnsupportedType},
		{name: "ext32", src: []byte{0xc9}, err: ErrUnsupportedType},
		{name: "fixext1", src: []byte{0xd4}, err: ErrUnsupportedType},
		{name: "fixext2", src: []byte{0xd5}, err: ErrUnsupportedType},
		{name: "fixext4", src: []byte{0xd6}, err: ErrUnsupportedType},
		{name: "fixext8", src: []byte{0xd7}, err: ErrUnsupportedType},
		{name: "fixext16", src: []byte{0xd8}, err: ErrUnsupportedType},
		{name: "false", src: []byte{0xc2}, err: ErrTypeMismatch},
		{name: "true", src: []byte{0xc3}, err: ErrTypeMismatch},
		{name: "positive fixint", src: []byte{0x00}, err: ErrTypeMismatch},
		{name: "negative fixint", src: []byte{0xff}, err: ErrTypeMismatch},
		{name: "incomplete uint16", src: []byte{0xcd}, err: ErrTypeMismatch},
		{name: "incomplete int64", src: []byte{0xd3}, err: ErrTypeMismatch},
		{name: "incomplete float32", src: []byte{0xca}, err: ErrTypeMismatch},
		{name: "incomplete float64", src: []byte{0xcb}, err: ErrTypeMismatch},
		{name: "fixstr", src: []byte{0xa1}, err: ErrTypeMismatch},
		{name: "incomplete str32", src: []byte{0xdb}, err: ErrTypeMismatch},
		{name: "incomplete bin32", src: []byte{0xc6}, err: ErrTypeMismatch},
		{name: "fixarray", src: []byte{0x90}, err: ErrTypeMismatch},
		{name: "incomplete array16", src: []byte{0xdc}, err: ErrTypeMismatch},
		{name: "fixmap", src: []byte{0x80}, err: ErrTypeMismatch},
		{name: "incomplete map32", src: []byte{0xdf}, err: ErrTypeMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before = append([]byte(nil), tt.src...)
				n      int
				err    error
			)

			sourceNil := bytes.NewReader(tt.src)
			err = NewReader(sourceNil).DecodeNil()
			n = len(tt.src) - sourceNil.Len()
			if err == nil && n != tt.n || !errors.Is(err, streamFuzzError(tt.err, len(tt.src))) {
				t.Fatalf("DecodeNil(%x) = (%d, %v), want (%d, %v)", tt.src, n, err, tt.n, tt.err)
			}

			if !bytes.Equal(tt.src, before) {
				t.Fatal("DecodeNil() modified source bytes")
			}
		})
	}
}

func TestReaderBoolFixtures(t *testing.T) {
	type testcase struct {
		name  string
		src   []byte
		value bool
		n     int
		err   error
	}
	tests := []testcase{
		{name: "false", src: []byte{0xc2}, n: 1},
		{name: "true", src: []byte{0xc3}, value: true, n: 1},
		{name: "false trailing true", src: []byte{0xc2, 0xc3}, n: 1},
		{name: "true trailing false", src: []byte{0xc3, 0xc2}, value: true, n: 1},
		{name: "trailing invalid tag", src: []byte{0xc3, 0xc1}, value: true, n: 1},
		{name: "nil input", err: errFuzzTruncated},
		{name: "empty input", src: []byte{}, err: errFuzzTruncated},
		{name: "reserved", src: []byte{0xc1}, err: ErrInvalidFormat},
		{name: "ext8", src: []byte{0xc7}, err: ErrUnsupportedType},
		{name: "ext16", src: []byte{0xc8}, err: ErrUnsupportedType},
		{name: "ext32", src: []byte{0xc9}, err: ErrUnsupportedType},
		{name: "fixext1", src: []byte{0xd4}, err: ErrUnsupportedType},
		{name: "fixext2", src: []byte{0xd5}, err: ErrUnsupportedType},
		{name: "fixext4", src: []byte{0xd6}, err: ErrUnsupportedType},
		{name: "fixext8", src: []byte{0xd7}, err: ErrUnsupportedType},
		{name: "fixext16", src: []byte{0xd8}, err: ErrUnsupportedType},
		{name: "nil", src: []byte{0xc0}, err: ErrTypeMismatch},
		{name: "positive fixint", src: []byte{0x00}, err: ErrTypeMismatch},
		{name: "negative fixint", src: []byte{0xff}, err: ErrTypeMismatch},
		{name: "incomplete uint16", src: []byte{0xcd}, err: ErrTypeMismatch},
		{name: "incomplete int64", src: []byte{0xd3}, err: ErrTypeMismatch},
		{name: "incomplete float32", src: []byte{0xca}, err: ErrTypeMismatch},
		{name: "incomplete float64", src: []byte{0xcb}, err: ErrTypeMismatch},
		{name: "fixstr", src: []byte{0xa1}, err: ErrTypeMismatch},
		{name: "incomplete str32", src: []byte{0xdb}, err: ErrTypeMismatch},
		{name: "incomplete bin32", src: []byte{0xc6}, err: ErrTypeMismatch},
		{name: "fixarray", src: []byte{0x90}, err: ErrTypeMismatch},
		{name: "incomplete array16", src: []byte{0xdc}, err: ErrTypeMismatch},
		{name: "fixmap", src: []byte{0x80}, err: ErrTypeMismatch},
		{name: "incomplete map32", src: []byte{0xdf}, err: ErrTypeMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before = append([]byte(nil), tt.src...)
				value  bool
				n      int
				err    error
			)

			sourceBool := bytes.NewReader(tt.src)
			value, err = NewReader(sourceBool).DecodeBool()
			n = len(tt.src) - sourceBool.Len()
			if value != tt.value || err == nil && n != tt.n || !errors.Is(err, streamFuzzError(tt.err, len(tt.src))) {
				t.Fatalf("DecodeBool(%x) = (%t, %d, %v), want (%t, %d, %v)", tt.src, value, n, err, tt.value, tt.n, tt.err)
			}

			if !bytes.Equal(tt.src, before) {
				t.Fatal("DecodeBool() modified source bytes")
			}
		})
	}
}

func TestReaderIntegersFixtures(t *testing.T) {
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
		{name: "nil input", signedErr: errFuzzTruncated, unsignedErr: errFuzzTruncated},
		{name: "empty input", src: []byte{}, signedErr: errFuzzTruncated, unsignedErr: errFuzzTruncated},
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
			var inputs = [][]byte{tt.src}
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

					sourceInt64 := bytes.NewReader(src)
					signed, signedErr = NewReader(sourceInt64).DecodeInt64()
					signedCount = len(src) - sourceInt64.Len()
					if signed != tt.signed || signedErr == nil && signedCount != wantSignedN || !errors.Is(signedErr, streamFuzzError(tt.signedErr, len(src))) {
						t.Fatalf("DecodeInt64(%x) = (%d, %d, %v), want (%d, %d, %v)", src, signed, signedCount, signedErr, tt.signed, wantSignedN, tt.signedErr)
					}

					if !bytes.Equal(src, before) {
						t.Fatal("DecodeInt64() modified source bytes")
					}

					sourceUint64 := bytes.NewReader(src)
					unsigned, unsignedErr = NewReader(sourceUint64).DecodeUint64()
					unsignedCount = len(src) - sourceUint64.Len()
					if unsigned != tt.unsigned || unsignedErr == nil && unsignedCount != wantUnsignedN || !errors.Is(unsignedErr, streamFuzzError(tt.unsignedErr, len(src))) {
						t.Fatalf("DecodeUint64(%x) = (%d, %d, %v), want (%d, %d, %v)", src, unsigned, unsignedCount, unsignedErr, tt.unsigned, wantUnsignedN, tt.unsignedErr)
					}

					if !bytes.Equal(src, before) {
						t.Fatal("DecodeUint64() modified source bytes")
					}
				})
			}
		})
	}
}

func TestReaderIntegersTruncatedFixtures(t *testing.T) {
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

					sourceInt64 := bytes.NewReader(src)
					signed, signedErr = NewReader(sourceInt64).DecodeInt64()
					signedCount = len(src) - sourceInt64.Len()
					if signed != 0 || !errors.Is(signedErr, streamFuzzError(errFuzzTruncated, len(src))) {
						t.Fatalf("DecodeInt64(%x) = (%d, %d, %v), want (0, 0, errFuzzTruncated)", src, signed, signedCount, signedErr)
					}

					sourceUint64 := bytes.NewReader(src)
					unsigned, unsignedErr = NewReader(sourceUint64).DecodeUint64()
					unsignedCount = len(src) - sourceUint64.Len()
					if unsigned != 0 || !errors.Is(unsignedErr, streamFuzzError(errFuzzTruncated, len(src))) {
						t.Fatalf("DecodeUint64(%x) = (%d, %d, %v), want (0, 0, errFuzzTruncated)", src, unsigned, unsignedCount, unsignedErr)
					}

					if !bytes.Equal(tt.src, before) {
						t.Fatal("integer reader modified source backing bytes")
					}
				})
			}
		})
	}
}

func TestReaderFloatsErrorsFixtures(t *testing.T) {
	type testcase struct {
		name string
		src  []byte
		err  error
	}
	tests := []testcase{
		{name: "nil input", err: errFuzzTruncated},
		{name: "empty input", src: []byte{}, err: errFuzzTruncated},
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

			sourceFloat32 := bytes.NewReader(tt.src)
			narrow, err = NewReader(sourceFloat32).DecodeFloat32()
			n = len(tt.src) - sourceFloat32.Len()
			if !errors.Is(err, streamFuzzError(tt.err, len(tt.src))) || math.Float32bits(narrow) != 0 {
				t.Fatalf("DecodeFloat32(%x) = (%v, %d, %v), want (0, 0, %v)", tt.src, narrow, n, err, tt.err)
			}

			if !bytes.Equal(tt.src, before) {
				t.Fatal("DecodeFloat32() modified source bytes")
			}

			sourceFloat64 := bytes.NewReader(tt.src)
			wide, err = NewReader(sourceFloat64).DecodeFloat64()
			n = len(tt.src) - sourceFloat64.Len()
			if !errors.Is(err, streamFuzzError(tt.err, len(tt.src))) || math.Float64bits(wide) != 0 {
				t.Fatalf("DecodeFloat64(%x) = (%v, %d, %v), want (0, 0, %v)", tt.src, wide, n, err, tt.err)
			}

			if !bytes.Equal(tt.src, before) {
				t.Fatal("DecodeFloat64() modified source bytes")
			}
		})
	}
}

func TestReaderFloatsTruncatedFixtures(t *testing.T) {
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
						narrowErr = errFuzzTruncated
						narrow    float32
						wide      float64
						n         int
						err       error
					)

					if length > 0 && tt.wire[0] == 0xcb {
						narrowErr = ErrTypeMismatch
					}

					sourceFloat32 := bytes.NewReader(src)
					narrow, err = NewReader(sourceFloat32).DecodeFloat32()
					n = len(src) - sourceFloat32.Len()
					if !errors.Is(err, streamFuzzError(narrowErr, len(src))) || math.Float32bits(narrow) != 0 {
						t.Fatalf("DecodeFloat32(%x) = (%v, %d, %v), want (0, 0, %v)", src, narrow, n, err, narrowErr)
					}

					sourceFloat64 := bytes.NewReader(src)
					wide, err = NewReader(sourceFloat64).DecodeFloat64()
					n = len(src) - sourceFloat64.Len()
					if !errors.Is(err, streamFuzzError(errFuzzTruncated, len(src))) || math.Float64bits(wide) != 0 {
						t.Fatalf("DecodeFloat64(%x) = (%v, %d, %v), want (0, 0, errFuzzTruncated)", src, wide, n, err)
					}

					if !bytes.Equal(tt.wire, before) {
						t.Fatal("float reader modified source backing bytes")
					}
				})
			}
		})
	}
}

func TestReaderArrayHeaderWiderFixtures(t *testing.T) {
	type testcase struct {
		name  string
		wire  []byte
		count uint32
	}
	tests := []testcase{
		{name: "array16 empty", wire: []byte{0xdc, 0x00, 0x00}},
		{name: "array32 empty", wire: []byte{0xdd, 0x00, 0x00, 0x00, 0x00}},
		{name: "array16 one", wire: []byte{0xdc, 0x00, 0x01}, count: 1},
		{name: "array32 one", wire: []byte{0xdd, 0x00, 0x00, 0x00, 0x01}, count: 1},
		{name: "array32 sixteen", wire: []byte{0xdd, 0x00, 0x00, 0x00, 0x10}, count: 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before = append([]byte(nil), tt.wire...)
				count  uint32
				n      int
				err    error
			)

			sourceArrayHeader := bytes.NewReader(tt.wire)
			count, err = NewReader(sourceArrayHeader).DecodeArrayHeader()
			n = len(tt.wire) - sourceArrayHeader.Len()
			if err != nil || count != tt.count || n != len(tt.wire) {
				t.Fatalf("DecodeArrayHeader(%x) = (%d, %d, %v)", tt.wire, count, n, err)
			}

			if !bytes.Equal(tt.wire, before) {
				t.Fatal("DecodeArrayHeader modified input")
			}
		})
	}
}

func TestReaderArrayHeaderErrorsFixtures(t *testing.T) {
	type testcase struct {
		name string
		src  []byte
		err  error
	}
	tests := []testcase{
		{name: "nil input", err: errFuzzTruncated},
		{name: "empty input", src: []byte{}, err: errFuzzTruncated},
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
		{name: "bool", src: []byte{0xc3}, err: ErrTypeMismatch},
		{name: "integer", src: []byte{0x01}, err: ErrTypeMismatch},
		{name: "negative integer", src: []byte{0xff}, err: ErrTypeMismatch},
		{name: "incomplete integer", src: []byte{0xcf}, err: ErrTypeMismatch},
		{name: "incomplete float", src: []byte{0xcb}, err: ErrTypeMismatch},
		{name: "string", src: []byte{0xa0}, err: ErrTypeMismatch},
		{name: "incomplete string", src: []byte{0xdb}, err: ErrTypeMismatch},
		{name: "binary", src: []byte{0xc4, 0x00}, err: ErrTypeMismatch},
		{name: "incomplete binary", src: []byte{0xc6}, err: ErrTypeMismatch},
		{name: "map", src: []byte{0x80}, err: ErrTypeMismatch},
		{name: "incomplete map16", src: []byte{0xde}, err: ErrTypeMismatch},
		{name: "incomplete map32", src: []byte{0xdf}, err: ErrTypeMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before = append([]byte(nil), tt.src...)
				count  uint32
				n      int
				err    error
			)

			sourceArrayHeader := bytes.NewReader(tt.src)
			count, err = NewReader(sourceArrayHeader).DecodeArrayHeader()
			n = len(tt.src) - sourceArrayHeader.Len()
			if count != 0 || !errors.Is(err, streamFuzzError(tt.err, len(tt.src))) {
				t.Fatalf("DecodeArrayHeader(%x) = (%d, %d, %v), want (0, 0, %v)", tt.src, count, n, err, tt.err)
			}

			if !bytes.Equal(tt.src, before) {
				t.Fatal("DecodeArrayHeader modified input")
			}
		})
	}
}

func TestReaderArrayHeaderTruncatedFixtures(t *testing.T) {
	tests := [][]byte{
		{0xdc, 0xff, 0xff},
		{0xdd, 0xff, 0xff, 0xff, 0xff},
	}
	for _, wire := range tests {
		for length := 0; length < len(wire); length++ {
			t.Run(fmt.Sprintf("tag_%x_length_%d", wire[0], length), func(t *testing.T) {
				var (
					before = append([]byte(nil), wire...)
					count  uint32
					n      int
					err    error
				)

				sourceArrayHeader := bytes.NewReader(wire[:length])
				count, err = NewReader(sourceArrayHeader).DecodeArrayHeader()
				n = len(wire[:length]) - sourceArrayHeader.Len()
				if count != 0 || !errors.Is(err, streamFuzzError(errFuzzTruncated, length)) {
					t.Fatalf("DecodeArrayHeader(%x) = (%d, %d, %v)", wire[:length], count, n, err)
				}

				if !bytes.Equal(wire, before) {
					t.Fatal("DecodeArrayHeader modified source backing storage")
				}
			})
		}
	}
}

func TestReaderArrayHeaderDoesNotValidateChildrenFixtures(t *testing.T) {
	for _, src := range [][]byte{{0x91}, {0x91, 0xc1}} {
		var (
			before = append([]byte(nil), src...)
			count  uint32
			n      int
			childN int
			err    error
			want   = errFuzzTruncated
		)

		sourceArrayHeader := bytes.NewReader(src)
		count, err = NewReader(sourceArrayHeader).DecodeArrayHeader()
		n = len(src) - sourceArrayHeader.Len()
		if err != nil || count != 1 || n != 1 {
			t.Fatalf("DecodeArrayHeader(%x) = (%d, %d, %v)", src, count, n, err)
		}

		if len(src) > n {
			want = ErrInvalidFormat
		}

		sourceNil := bytes.NewReader(src[n:])
		err = NewReader(sourceNil).DecodeNil()
		childN = len(src[n:]) - sourceNil.Len()
		if !errors.Is(err, streamFuzzError(want, len(src[n:]))) {
			t.Fatalf("DecodeNil(child) = (%d, %v), want (0, %v)", childN, err, want)
		}

		if !bytes.Equal(src, before) {
			t.Fatal("header or child read modified input")
		}
	}
}

func TestReaderMapHeaderWiderFixtures(t *testing.T) {
	type testcase struct {
		name  string
		wire  []byte
		count uint32
	}
	tests := []testcase{
		{name: "map16 empty", wire: []byte{0xde, 0x00, 0x00}},
		{name: "map32 empty", wire: []byte{0xdf, 0x00, 0x00, 0x00, 0x00}},
		{name: "map16 one", wire: []byte{0xde, 0x00, 0x01}, count: 1},
		{name: "map32 one", wire: []byte{0xdf, 0x00, 0x00, 0x00, 0x01}, count: 1},
		{name: "map32 sixteen", wire: []byte{0xdf, 0x00, 0x00, 0x00, 0x10}, count: 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before = append([]byte(nil), tt.wire...)
				count  uint32
				n      int
				err    error
			)

			sourceMapHeader := bytes.NewReader(tt.wire)
			count, err = NewReader(sourceMapHeader).DecodeMapHeader()
			n = len(tt.wire) - sourceMapHeader.Len()
			if err != nil || count != tt.count || n != len(tt.wire) {
				t.Fatalf("DecodeMapHeader(%x) = (%d, %d, %v)", tt.wire, count, n, err)
			}

			if !bytes.Equal(tt.wire, before) {
				t.Fatal("DecodeMapHeader modified input")
			}
		})
	}
}

func TestReaderMapHeaderErrorsFixtures(t *testing.T) {
	type testcase struct {
		name string
		src  []byte
		err  error
	}
	tests := []testcase{
		{name: "nil input", err: errFuzzTruncated},
		{name: "empty input", src: []byte{}, err: errFuzzTruncated},
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
		{name: "bool", src: []byte{0xc3}, err: ErrTypeMismatch},
		{name: "integer", src: []byte{0x01}, err: ErrTypeMismatch},
		{name: "negative integer", src: []byte{0xff}, err: ErrTypeMismatch},
		{name: "incomplete integer", src: []byte{0xcf}, err: ErrTypeMismatch},
		{name: "incomplete float", src: []byte{0xcb}, err: ErrTypeMismatch},
		{name: "string", src: []byte{0xa0}, err: ErrTypeMismatch},
		{name: "incomplete string", src: []byte{0xdb}, err: ErrTypeMismatch},
		{name: "binary", src: []byte{0xc4, 0x00}, err: ErrTypeMismatch},
		{name: "incomplete binary", src: []byte{0xc6}, err: ErrTypeMismatch},
		{name: "array", src: []byte{0x90}, err: ErrTypeMismatch},
		{name: "incomplete array16", src: []byte{0xdc}, err: ErrTypeMismatch},
		{name: "incomplete array32", src: []byte{0xdd}, err: ErrTypeMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before = append([]byte(nil), tt.src...)
				count  uint32
				n      int
				err    error
			)

			sourceMapHeader := bytes.NewReader(tt.src)
			count, err = NewReader(sourceMapHeader).DecodeMapHeader()
			n = len(tt.src) - sourceMapHeader.Len()
			if count != 0 || !errors.Is(err, streamFuzzError(tt.err, len(tt.src))) {
				t.Fatalf("DecodeMapHeader(%x) = (%d, %d, %v), want (0, 0, %v)", tt.src, count, n, err, tt.err)
			}

			if !bytes.Equal(tt.src, before) {
				t.Fatal("DecodeMapHeader modified input")
			}
		})
	}
}

func TestReaderMapHeaderTruncatedFixtures(t *testing.T) {
	tests := [][]byte{
		{0xde, 0xff, 0xff},
		{0xdf, 0xff, 0xff, 0xff, 0xff},
	}
	for _, wire := range tests {
		for length := 0; length < len(wire); length++ {
			t.Run(fmt.Sprintf("tag_%x_length_%d", wire[0], length), func(t *testing.T) {
				var (
					before = append([]byte(nil), wire...)
					count  uint32
					n      int
					err    error
				)

				sourceMapHeader := bytes.NewReader(wire[:length])
				count, err = NewReader(sourceMapHeader).DecodeMapHeader()
				n = len(wire[:length]) - sourceMapHeader.Len()
				if count != 0 || !errors.Is(err, streamFuzzError(errFuzzTruncated, length)) {
					t.Fatalf("DecodeMapHeader(%x) = (%d, %d, %v)", wire[:length], count, n, err)
				}

				if !bytes.Equal(wire, before) {
					t.Fatal("DecodeMapHeader modified source backing storage")
				}
			})
		}
	}
}

func TestReaderMapHeaderDoesNotValidateChildrenFixtures(t *testing.T) {
	for _, src := range [][]byte{{0x81}, {0x81, 0xc1}} {
		var (
			before = append([]byte(nil), src...)
			count  uint32
			n      int
			childN int
			err    error
			want   = errFuzzTruncated
		)

		sourceMapHeader := bytes.NewReader(src)
		count, err = NewReader(sourceMapHeader).DecodeMapHeader()
		n = len(src) - sourceMapHeader.Len()
		if err != nil || count != 1 || n != 1 {
			t.Fatalf("DecodeMapHeader(%x) = (%d, %d, %v)", src, count, n, err)
		}

		if len(src) > n {
			want = ErrInvalidFormat
		}

		sourceNil := bytes.NewReader(src[n:])
		err = NewReader(sourceNil).DecodeNil()
		childN = len(src[n:]) - sourceNil.Len()
		if !errors.Is(err, streamFuzzError(want, len(src[n:]))) {
			t.Fatalf("DecodeNil(child) = (%d, %v), want (0, %v)", childN, err, want)
		}

		if !bytes.Equal(src, before) {
			t.Fatal("header or child read modified input")
		}
	}
}

func TestReaderStringWiderHeadersFixtures(t *testing.T) {
	type testcase struct {
		name string
		src  []byte
		want string
	}
	tests := []testcase{
		{name: "str8 empty", src: []byte{0xd9, 0x00}},
		{name: "str16 empty", src: []byte{0xda, 0x00, 0x00}},
		{name: "str32 empty", src: []byte{0xdb, 0x00, 0x00, 0x00, 0x00}},
		{name: "str8 one", src: []byte{0xd9, 0x01, 0x78}, want: "x"},
		{name: "str16 one", src: []byte{0xda, 0x00, 0x01, 0x78}, want: "x"},
		{name: "str32 one", src: []byte{0xdb, 0x00, 0x00, 0x00, 0x01, 0x78}, want: "x"},
		{name: "raw bytes", src: []byte{0xa4, 0x00, 0xc3, 0xa9, 0xff}, want: "\x00é\xff"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				owned string
				n     int
				err   error
			)

			sourceString := bytes.NewReader(tt.src)
			owned, err = NewReader(sourceString).DecodeString()
			n = len(tt.src) - sourceString.Len()
			if err != nil || owned != tt.want || n != len(tt.src) {
				t.Fatalf("DecodeString(%x) = (%q, %d, %v)", tt.src, owned, n, err)
			}
		})
	}
}

func TestReaderStringErrorsFixtures(t *testing.T) {
	type testcase struct {
		name string
		src  []byte
		err  error
	}
	tests := []testcase{
		{name: "nil input", err: errFuzzTruncated},
		{name: "empty input", src: []byte{}, err: errFuzzTruncated},
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
		{name: "bool", src: []byte{0xc3}, err: ErrTypeMismatch},
		{name: "integer", src: []byte{0x01}, err: ErrTypeMismatch},
		{name: "negative integer", src: []byte{0xff}, err: ErrTypeMismatch},
		{name: "incomplete integer", src: []byte{0xcf}, err: ErrTypeMismatch},
		{name: "incomplete float", src: []byte{0xcb}, err: ErrTypeMismatch},
		{name: "empty binary", src: []byte{0xc4, 0x00}, err: ErrTypeMismatch},
		{name: "incomplete binary", src: []byte{0xc6}, err: ErrTypeMismatch},
		{name: "array", src: []byte{0x90}, err: ErrTypeMismatch},
		{name: "incomplete array", src: []byte{0xdc}, err: ErrTypeMismatch},
		{name: "map", src: []byte{0x80}, err: ErrTypeMismatch},
		{name: "incomplete map", src: []byte{0xdf}, err: ErrTypeMismatch},
		{name: "huge below max int32", src: []byte{0xdb, 0x7f, 0xff, 0xff, 0xff, 0x78}, err: ErrLimitExceeded},
		{name: "huge above max int32", src: []byte{0xdb, 0x80, 0x00, 0x00, 0x00, 0x78}, err: ErrLimitExceeded},
		{name: "maximum wire length", src: []byte{0xdb, 0xff, 0xff, 0xff, 0xff, 0x78}, err: ErrLimitExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before = append([]byte(nil), tt.src...)
				owned  string
				n      int
				err    error
			)

			sourceString := bytes.NewReader(tt.src)
			owned, err = NewReader(sourceString).DecodeString()
			n = len(tt.src) - sourceString.Len()
			if owned != "" || !errors.Is(err, streamFuzzError(tt.err, len(tt.src))) {
				t.Fatalf("DecodeString(%x) = (%q, %d, %v), want zero and %v", tt.src, owned, n, err, tt.err)
			}

			if !bytes.Equal(tt.src, before) {
				t.Fatal("string reader modified input")
			}
		})
	}
}

func TestReaderStringsTruncatedFixtures(t *testing.T) {
	tests := [][]byte{
		{0xa2, 0x78, 0x79},
		{0xd9, 0x02, 0x78, 0x79},
		{0xda, 0x00, 0x02, 0x78, 0x79},
		{0xdb, 0x00, 0x00, 0x00, 0x02, 0x78, 0x79},
	}
	for _, wire := range tests {
		for length := 0; length < len(wire); length++ {
			t.Run(fmt.Sprintf("tag_%x_length_%d", wire[0], length), func(t *testing.T) {
				var (
					src    = wire[:length]
					before = append([]byte(nil), wire...)
					owned  string
					n      int
					err    error
				)

				sourceString := bytes.NewReader(src)
				owned, err = NewReader(sourceString).DecodeString()
				n = len(src) - sourceString.Len()
				if owned != "" || !errors.Is(err, streamFuzzError(errFuzzTruncated, len(src))) {
					t.Fatalf("DecodeString(%x) = (%q, %d, %v)", src, owned, n, err)
				}

				if !bytes.Equal(wire, before) {
					t.Fatal("string reader modified source backing storage")
				}
			})
		}
	}
}

func TestReadererStringOwnershipFixtureFixtures(t *testing.T) {
	var (
		src        = []byte{0xa3, 'a', 'b', 'c'}
		r          = NewReader(bytes.NewReader(src))
		owned, err = r.DecodeString()
	)

	if err != nil || owned != "abc" {
		t.Fatalf("DecodeString() = (%q, %v)", owned, err)
	}

	src[1] = 'z'
	r.Reset(bytes.NewReader([]byte{0xa1, 'x'}))
	if _, err = r.DecodeString(); err != nil || owned != "abc" {
		t.Fatalf("owned string changed after source mutation and Reset: %q, %v", owned, err)
	}
}

func TestReaderBytesWiderHeadersFixtures(t *testing.T) {
	type testcase struct {
		name string
		src  []byte
		want []byte
	}
	tests := []testcase{
		{name: "bin16 empty", src: []byte{0xc5, 0x00, 0x00}},
		{name: "bin32 empty", src: []byte{0xc6, 0x00, 0x00, 0x00, 0x00}},
		{name: "bin16 one", src: []byte{0xc5, 0x00, 0x01, 0x78}, want: []byte{0x78}},
		{name: "bin32 one", src: []byte{0xc6, 0x00, 0x00, 0x00, 0x01, 0x78}, want: []byte{0x78}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				view []byte
				n    int
				err  error
			)

			sourceBytes := bytes.NewReader(tt.src)
			view, err = NewReader(sourceBytes).DecodeBytes()
			n = len(tt.src) - sourceBytes.Len()
			if err != nil || !bytes.Equal(view, tt.want) || n != len(tt.src) || cap(view) != len(view) {
				t.Fatalf("DecodeBytes(%x) = (%x, %d, %v), cap %d", tt.src, view, n, err, cap(view))
			}
		})
	}
}

func TestReaderBytesErrorsFixtures(t *testing.T) {
	type testcase struct {
		name string
		src  []byte
		err  error
	}
	tests := []testcase{
		{name: "nil input", err: errFuzzTruncated},
		{name: "empty input", src: []byte{}, err: errFuzzTruncated},
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
		{name: "bool", src: []byte{0xc2}, err: ErrTypeMismatch},
		{name: "integer", src: []byte{0x01}, err: ErrTypeMismatch},
		{name: "negative integer", src: []byte{0xff}, err: ErrTypeMismatch},
		{name: "incomplete integer", src: []byte{0xcd}, err: ErrTypeMismatch},
		{name: "incomplete float", src: []byte{0xca}, err: ErrTypeMismatch},
		{name: "empty string", src: []byte{0xa0}, err: ErrTypeMismatch},
		{name: "incomplete string", src: []byte{0xdb}, err: ErrTypeMismatch},
		{name: "array", src: []byte{0x90}, err: ErrTypeMismatch},
		{name: "incomplete array", src: []byte{0xdd}, err: ErrTypeMismatch},
		{name: "map", src: []byte{0x80}, err: ErrTypeMismatch},
		{name: "incomplete map", src: []byte{0xde}, err: ErrTypeMismatch},
		{name: "huge below max int32", src: []byte{0xc6, 0x7f, 0xff, 0xff, 0xff, 0x78}, err: ErrLimitExceeded},
		{name: "huge above max int32", src: []byte{0xc6, 0x80, 0x00, 0x00, 0x00, 0x78}, err: ErrLimitExceeded},
		{name: "maximum wire length", src: []byte{0xc6, 0xff, 0xff, 0xff, 0xff, 0x78}, err: ErrLimitExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before = append([]byte(nil), tt.src...)
				view   []byte
				n      int
				err    error
			)

			sourceBytes := bytes.NewReader(tt.src)
			view, err = NewReader(sourceBytes).DecodeBytes()
			n = len(tt.src) - sourceBytes.Len()
			if view != nil || !errors.Is(err, streamFuzzError(tt.err, len(tt.src))) {
				t.Fatalf("DecodeBytes(%x) = (%x, %d, %v), want nil, 0, %v", tt.src, view, n, err, tt.err)
			}

			if !bytes.Equal(tt.src, before) {
				t.Fatal("DecodeBytes modified input")
			}
		})
	}
}

func TestReaderBytesTruncatedFixtures(t *testing.T) {
	tests := [][]byte{
		{0xc4, 0x02, 0x78, 0x79},
		{0xc5, 0x00, 0x02, 0x78, 0x79},
		{0xc6, 0x00, 0x00, 0x00, 0x02, 0x78, 0x79},
	}
	for _, wire := range tests {
		for length := 0; length < len(wire); length++ {
			t.Run(fmt.Sprintf("tag_%x_length_%d", wire[0], length), func(t *testing.T) {
				var (
					src    = wire[:length]
					before = append([]byte(nil), wire...)
					view   []byte
					n      int
					err    error
				)

				sourceBytes := bytes.NewReader(src)
				view, err = NewReader(sourceBytes).DecodeBytes()
				n = len(src) - sourceBytes.Len()
				if view != nil || !errors.Is(err, streamFuzzError(errFuzzTruncated, len(src))) {
					t.Fatalf("DecodeBytes(%x) = (%x, %d, %v)", src, view, n, err)
				}

				if !bytes.Equal(wire, before) {
					t.Fatal("DecodeBytes modified source backing storage")
				}
			})
		}
	}
}

func TestReadererBytesOwnershipFixtureFixtures(t *testing.T) {
	var (
		src        = []byte{0xc4, 3, 'a', 'b', 'c'}
		r          = NewReader(bytes.NewReader(src))
		owned, err = r.DecodeBytes()
	)

	if err != nil || string(owned) != "abc" {
		t.Fatalf("DecodeBytes() = (%x, %v)", owned, err)
	}

	src[2] = 'z'
	r.Reset(bytes.NewReader([]byte{0xc4, 1, 'x'}))
	if _, err = r.DecodeBytes(); err != nil || string(owned) != "abc" {
		t.Fatalf("owned bytes changed after source mutation and Reset: %x, %v", owned, err)
	}

	owned[1] = 'y'
	if src[3] != 'b' {
		t.Fatal("owned payload mutation changed source storage")
	}
}

func TestReaderZeroAllocations(t *testing.T) {
	var (
		wire = []byte{0xc0, 0xc3, 0xd2, 0xff, 0xff, 0x7f, 0xff,
			0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			0xca, 0x3f, 0xc0, 0, 0, 0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0,
			0xdd, 0, 1, 0, 0, 0xdf, 0, 1, 0, 0}
		input bytes.Reader
		r     = NewReader(&input)
	)

	allocations := testing.AllocsPerRun(1000, func() {
		input.Reset(wire)
		r.Reset(&input)
		var err error
		if err = r.DecodeNil(); err != nil {
			t.Fatal(err)
		}

		if benchmarkBoolSink, err = r.DecodeBool(); err != nil {
			t.Fatal(err)
		}

		if benchmarkIntSink, err = r.DecodeInt64(); err != nil {
			t.Fatal(err)
		}

		if benchmarkUintSink, err = r.DecodeUint64(); err != nil {
			t.Fatal(err)
		}

		if benchmarkFloat32Sink, err = r.DecodeFloat32(); err != nil {
			t.Fatal(err)
		}

		if benchmarkFloat64Sink, err = r.DecodeFloat64(); err != nil {
			t.Fatal(err)
		}

		if benchmarkHeaderSink, err = r.DecodeArrayHeader(); err != nil {
			t.Fatal(err)
		}

		if benchmarkHeaderSink, err = r.DecodeMapHeader(); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("reused Reader allocated %g times per run, want zero", allocations)
	}
}

func TestReaderInitializationAndReset(t *testing.T) {
	var (
		zero  Reader
		tests = []struct {
			name string
			d    *Reader
		}{
			{name: "zero value", d: &zero},
			{name: "nil constructor", d: NewReader(nil)},
		}
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireReaderScalarFailure(t, tt.d, ErrUninitialized)
			if tt.d.state != nil && tt.d.state.err != nil {
				t.Fatal("uninitialized error became terminal")
			}

			r := &decoderTestReader{data: []byte{0xc3}, endErr: io.EOF}
			tt.d.Reset(r)
			if r.reads != 0 {
				t.Fatal("Reset read input")
			}

			var value, err = tt.d.DecodeBool()
			if !value || err != nil {
				t.Fatalf("first attached decode = (%v, %v)", value, err)
			}

			tt.d.Reset(bytes.NewReader([]byte{0xc0}))
			if err = tt.d.DecodeNil(); err != nil {
				t.Fatalf("Reset did not clear pending EOF: %v", err)
			}

			if r.closes != 0 || r.reads != 1 {
				t.Fatal("Reset closed or drained previous reader")
			}

			err = tt.d.DecodeNil()
			if !errors.Is(err, io.EOF) {
				t.Fatalf("expected terminal EOF, got %v", err)
			}

			tt.d.Reset(nil)
			requireReaderScalarFailure(t, tt.d, ErrUninitialized)
			tt.d.Reset(bytes.NewReader([]byte{0x07}))
			integer, integerErr := tt.d.DecodeUint64()
			if integer != 7 || integerErr != nil {
				t.Fatalf("Reset after failure = (%d, %v)", integer, integerErr)
			}
		})
	}
}

func TestReaderTypedNilReader(t *testing.T) {
	var reader *decoderTestReader
	d := NewReader(reader)
	value, err := d.DecodeBool()
	if !value || err != nil {
		t.Fatalf("typed nil reader was not called: (%v, %v)", value, err)
	}
}

func TestReaderLimits(t *testing.T) {
	var (
		defaults = ReaderLimits{MaxPayloadBytes: 16777216, MaxSkipDepth: 64,
			MaxSkipValues: 1048576, MaxSkipBytes: 67108864}
		custom = ReaderLimits{MaxPayloadBytes: 1, MaxSkipDepth: 256,
			MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64}
		zero Reader
	)

	zero.Reset(nil)
	if zero.state.limits != defaults || NewReader(nil).state.limits != defaults {
		t.Fatal("constructor/zero Reset did not initialize default limits")
	}

	var unattached Reader
	if err := unattached.SetLimits(custom); err != nil || unattached.state.limits != custom {
		t.Fatalf("unattached SetLimits = %v, limits %+v", err, unattached.state.limits)
	}

	unattached.Reset(bytes.NewReader([]byte{0xc3}))
	if unattached.state.limits != custom {
		t.Fatal("Reset discarded configured limits")
	}

	tests := []struct {
		name   string
		limits ReaderLimits
		want   ReaderLimits
	}{
		{name: "all defaults", want: defaults},
		{name: "small payload", limits: ReaderLimits{MaxPayloadBytes: 2},
			want: ReaderLimits{MaxPayloadBytes: 2, MaxSkipDepth: 64, MaxSkipValues: 1048576, MaxSkipBytes: 67108864}},
		{name: "wire and work maximum", limits: ReaderLimits{MaxPayloadBytes: 4294967295,
			MaxSkipDepth: 256, MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64},
			want: ReaderLimits{MaxPayloadBytes: 4294967295, MaxSkipDepth: 256,
				MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &decoderTestReader{data: []byte{0xc3}}
			d := NewReader(r)
			if err := d.SetLimits(tt.limits); err != nil || d.state.limits != tt.want || r.reads != 0 {
				t.Fatalf("SetLimits = %v, %+v, reads %d; want %+v", err, d.state.limits, r.reads, tt.want)
			}

			d.Reset(nil)
			if d.state.limits != tt.want {
				t.Fatal("nil Reset changed configured limits")
			}
		})
	}
	for _, invalid := range []ReaderLimits{{MaxPayloadBytes: 4294967296}, {MaxSkipDepth: 257}} {
		if err := unattached.SetLimits(invalid); err != ErrInvalidArgument || unattached.state.limits != custom {
			t.Fatalf("invalid limits = (%+v, %v), previous %+v", unattached.state.limits, err, custom)
		}
	}
}

func TestReaderSetLimitsPreservesFailure(t *testing.T) {
	var (
		r       = &decoderTestReader{data: []byte{0xc1, 0xc3}}
		d       = NewReader(r)
		failure = d.DecodeNil()
		reads   = r.reads
	)

	if failure != ErrInvalidFormat {
		t.Fatalf("initial failure = %v", failure)
	}

	if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: 1}); err != nil {
		t.Fatal(err)
	}

	if err := d.SetLimits(ReaderLimits{MaxSkipDepth: 257}); err != ErrInvalidArgument {
		t.Fatalf("invalid SetLimits after failure = %v", err)
	}

	requireReaderScalarFailure(t, d, failure)
	if r.reads != reads || d.state.limits.MaxPayloadBytes != 1 {
		t.Fatal("SetLimits performed I/O, cleared failure, or partially applied invalid limits")
	}

	d.Reset(bytes.NewReader([]byte{0xc3}))
	value, err := d.DecodeBool()
	if !value || err != nil || d.state.limits.MaxPayloadBytes != 1 {
		t.Fatalf("Reset after SetLimits = (%v, %v), limits %+v", value, err, d.state.limits)
	}
}

func TestReaderCopiesShareEOFResetAndLimits(t *testing.T) {
	var (
		input      = &decoderTestReader{data: []byte{0xc3}, endErr: io.EOF}
		r          = NewReader(input)
		copied     = *r
		value, err = copied.DecodeBool()
		first      error
	)

	if err != nil || !value {
		t.Fatalf("copied DecodeBool = (%v, %v)", value, err)
	}

	first = r.DecodeNil()
	if !errors.Is(first, io.EOF) || input.reads != 1 {
		t.Fatalf("shared pending EOF = %v, reads %d", first, input.reads)
	}

	if err = copied.SetLimits(ReaderLimits{MaxPayloadBytes: 1}); err != nil {
		t.Fatal(err)
	}

	if err = copied.DecodeNil(); err != first || input.reads != 1 {
		t.Fatalf("copied terminal error = %v, reads %d", err, input.reads)
	}

	r.Reset(bytes.NewReader([]byte{0xa2, 'i', 'd'}))
	if _, err = copied.DecodeString(); err != ErrLimitExceeded {
		t.Fatalf("shared reset and limits = %v, want ErrLimitExceeded", err)
	}

	copied.Reset(bytes.NewReader([]byte{0xc0}))
	if err = r.DecodeNil(); err != nil || input.closes != 0 {
		t.Fatalf("DecodeNil after copied Reset = %v, closes %d", err, input.closes)
	}
}

func TestReaderPayloadWire(t *testing.T) {
	tests := []struct {
		name         string
		length       int
		stringHeader []byte
		binaryHeader []byte
	}{
		{name: "empty", stringHeader: []byte{0xa0}, binaryHeader: []byte{0xc4, 0}},
		{name: "one", length: 1, stringHeader: []byte{0xa1}, binaryHeader: []byte{0xc4, 1}},
		{name: "8", length: 8, stringHeader: []byte{0xa8}, binaryHeader: []byte{0xc4, 8}},
		{name: "9", length: 9, stringHeader: []byte{0xa9}, binaryHeader: []byte{0xc4, 9}},
		{name: "10", length: 10, stringHeader: []byte{0xaa}, binaryHeader: []byte{0xc4, 10}},
		{name: "31", length: 31, stringHeader: []byte{0xbf}, binaryHeader: []byte{0xc4, 31}},
		{name: "32", length: 32, stringHeader: []byte{0xd9, 32}, binaryHeader: []byte{0xc4, 32}},
		{name: "255", length: 255, stringHeader: []byte{0xd9, 0xff}, binaryHeader: []byte{0xc4, 0xff}},
		{name: "256", length: 256, stringHeader: []byte{0xda, 1, 0}, binaryHeader: []byte{0xc5, 1, 0}},
		{name: "65535", length: 65535, stringHeader: []byte{0xda, 0xff, 0xff}, binaryHeader: []byte{0xc5, 0xff, 0xff}},
		{name: "65536", length: 65536, stringHeader: []byte{0xdb, 0, 1, 0, 0}, binaryHeader: []byte{0xc6, 0, 1, 0, 0}},
	}
	for _, tt := range tests {
		value := bytes.Repeat([]byte{0xff}, tt.length)
		families := []struct {
			name   string
			header []byte
			owned  func(d *Reader) (out []byte, err error)
			copy   func(d *Reader, dst io.Writer) (n uint64, err error)
		}{
			{name: "string", header: tt.stringHeader, owned: func(d *Reader) (out []byte, err error) {
				var text string
				text, err = d.DecodeString()
				return []byte(text), err
			}, copy: (*Reader).DecodeStringTo},
			{name: "binary", header: tt.binaryHeader, owned: (*Reader).DecodeBytes, copy: (*Reader).DecodeBytesTo},
		}
		for _, family := range families {
			t.Run(tt.name+"/"+family.name, func(t *testing.T) {
				var (
					wire = append(append(append([]byte(nil), family.header...), value...), 0xc3)
					r    = &decoderTestReader{data: wire, maxRead: 7}
					d    = NewReader(r)
				)

				owned, err := family.owned(d)
				if err != nil || owned == nil || !bytes.Equal(owned, value) || r.position != len(wire)-1 {
					t.Fatalf("owned result = (%d bytes, %v), consumed %d", len(owned), err, r.position)
				}

				for _, request := range r.requests {
					if request > 32768 {
						t.Fatalf("owned payload request = %d, exceeds 32 KiB", request)
					}
				}
				if next, nextErr := d.DecodeBool(); !next || nextErr != nil {
					t.Fatalf("following bool = (%v, %v)", next, nextErr)
				}

				r = &decoderTestReader{data: wire, maxRead: 7}
				d.Reset(r)
				var dst bytes.Buffer
				n, transferErr := family.copy(d, &dst)
				if transferErr != nil || n != uint64(tt.length) || !bytes.Equal(dst.Bytes(), value) || r.position != len(wire)-1 {
					t.Fatalf("transfer = (%d, %v), consumed %d", n, transferErr, r.position)
				}

				if next, nextErr := d.DecodeBool(); !next || nextErr != nil {
					t.Fatalf("following transfer bool = (%v, %v)", next, nextErr)
				}
			})
		}
	}
}

func TestReaderPayloadWiderHeadersAndRawBytes(t *testing.T) {
	tests := []struct {
		name     string
		wire     []byte
		isString bool
		want     []byte
	}{
		{name: "str8 empty", wire: []byte{0xd9, 0}, isString: true},
		{name: "str16 empty", wire: []byte{0xda, 0, 0}, isString: true},
		{name: "str32 empty", wire: []byte{0xdb, 0, 0, 0, 0}, isString: true},
		{name: "bin16 empty", wire: []byte{0xc5, 0, 0}},
		{name: "bin32 empty", wire: []byte{0xc6, 0, 0, 0, 0}},
		{name: "str8 one", wire: []byte{0xd9, 1, 0x78}, isString: true, want: []byte{0x78}},
		{name: "str16 one", wire: []byte{0xda, 0, 1, 0x78}, isString: true, want: []byte{0x78}},
		{name: "str32 one", wire: []byte{0xdb, 0, 0, 0, 1, 0x78}, isString: true, want: []byte{0x78}},
		{name: "bin16 one", wire: []byte{0xc5, 0, 1, 0x78}, want: []byte{0x78}},
		{name: "bin32 one", wire: []byte{0xc6, 0, 0, 0, 1, 0x78}, want: []byte{0x78}},
		{name: "raw string", wire: []byte{0xa4, 0, 0xc3, 0xa9, 0xff}, isString: true, want: []byte{0, 0xc3, 0xa9, 0xff}},
		{name: "raw binary", wire: []byte{0xc4, 4, 0, 0xc3, 0xa9, 0xff}, want: []byte{0, 0xc3, 0xa9, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r     = &decoderTestReader{data: tt.wire, maxRead: 1, endErr: io.EOF}
				d     = NewReader(r)
				owned []byte
				err   error
			)

			if tt.isString {
				var text string
				text, err = d.DecodeString()
				owned = []byte(text)
			} else {
				owned, err = d.DecodeBytes()
			}

			if err != nil || owned == nil || !bytes.Equal(owned, tt.want) {
				t.Fatalf("owned = (%x, %v), want %x", owned, err, tt.want)
			}

			reads := r.reads
			if nextErr := d.DecodeNil(); !errors.Is(nextErr, io.EOF) || r.reads != reads {
				t.Fatalf("pending EOF = %v, reads %d, want %d", nextErr, r.reads, reads)
			}

			r = &decoderTestReader{data: tt.wire, maxRead: 1, endErr: io.EOF}
			d.Reset(r)
			var (
				dst bytes.Buffer
				n   uint64
			)

			if tt.isString {
				n, err = d.DecodeStringTo(&dst)
			} else {
				n, err = d.DecodeBytesTo(&dst)
			}

			if err != nil || n != uint64(len(tt.want)) || !bytes.Equal(dst.Bytes(), tt.want) {
				t.Fatalf("transfer = (%d, %v), output %x", n, err, dst.Bytes())
			}
		})
	}
}

func TestReaderPayloadTruncation(t *testing.T) {
	tests := []struct {
		wire     []byte
		header   int
		isString bool
	}{
		{wire: []byte{0xa2, 1, 2}, header: 1, isString: true},
		{wire: []byte{0xa9, 1, 2, 3, 4, 5, 6, 7, 8, 9}, header: 1, isString: true},
		{wire: []byte{0xaa, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, header: 1, isString: true},
		{wire: []byte{0xd9, 2, 1, 2}, header: 2, isString: true},
		{wire: []byte{0xda, 0, 2, 1, 2}, header: 3, isString: true},
		{wire: []byte{0xdb, 0, 0, 0, 2, 1, 2}, header: 5, isString: true},
		{wire: []byte{0xc4, 2, 1, 2}, header: 2},
		{wire: []byte{0xc5, 0, 2, 1, 2}, header: 3},
		{wire: []byte{0xc6, 0, 0, 0, 2, 1, 2}, header: 5},
	}
	for _, tt := range tests {
		for length := 0; length < len(tt.wire); length++ {
			t.Run(fmt.Sprintf("tag_%x/length_%d", tt.wire[0], length), func(t *testing.T) {
				var (
					r     = &decoderTestReader{data: tt.wire[:length], maxRead: 1}
					d     = NewReader(r)
					want  = io.ErrUnexpectedEOF
					owned []byte
					err   error
				)

				if length == 0 {
					want = io.EOF
				}

				if tt.isString {
					var text string
					text, err = d.DecodeString()
					if text != "" {
						t.Fatal("failed string is not empty")
					}
				} else {
					owned, err = d.DecodeBytes()
				}

				if owned != nil || !errors.Is(err, want) || errors.Is(err, errFuzzTruncated) || r.position != length {
					t.Fatalf("owned truncation = (%x, %v), consumed %d", owned, err, r.position)
				}

				requireReaderPayloadFailure(t, d, err)
				r = &decoderTestReader{data: tt.wire[:length], maxRead: 1}
				d.Reset(r)
				var (
					dst   bytes.Buffer
					n     uint64
					wantN int
				)

				if length > tt.header {
					wantN = length - tt.header
				}

				if tt.isString {
					n, err = d.DecodeStringTo(&dst)
				} else {
					n, err = d.DecodeBytesTo(&dst)
				}

				if n != uint64(wantN) || dst.Len() != wantN || !errors.Is(err, want) || errors.Is(err, errFuzzTruncated) || r.position != length {
					t.Fatalf("transfer truncation = (%d, %v), bytes %d, consumed %d", n, err, dst.Len(), r.position)
				}

				reads := r.reads
				requireReaderPayloadFailure(t, d, err)
				if r.reads != reads {
					t.Fatal("sticky payload failure performed more reads")
				}
			})
		}
	}
}

func requireReaderPayloadFailure(t *testing.T, d *Reader, want error) {
	t.Helper()
	text, textErr := d.DecodeString()
	if text != "" || textErr != want {
		t.Fatalf("DecodeString after failure = (%q, %v)", text, textErr)
	}

	value, bytesErr := d.DecodeBytes()
	if value != nil || bytesErr != want {
		t.Fatalf("DecodeBytes after failure = (%x, %v)", value, bytesErr)
	}

	for _, transfer := range []func(io.Writer) (uint64, error){d.DecodeStringTo, d.DecodeBytesTo} {
		if n, err := transfer(nil); n != 0 || err != want {
			t.Fatalf("nil transfer after failure = (%d, %v)", n, err)
		}
	}
	requireReaderScalarFailure(t, d, want)
}

func TestReaderPayloadLimits(t *testing.T) {
	for _, header := range [][]byte{{0xdb, 0xff, 0xff, 0xff, 0xff}, {0xc6, 0xff, 0xff, 0xff, 0xff}} {
		r := &decoderTestReader{data: append(append([]byte(nil), header...), 0x78)}
		d := NewReader(r)
		var err error
		if header[0] == 0xdb {
			_, err = d.DecodeString()
		} else {
			_, err = d.DecodeBytes()
		}

		if err != ErrLimitExceeded || r.position != len(header) || d.state.buffer != nil {
			t.Fatalf("hostile length = %v, consumed %d, buffer %d", err, r.position, len(d.state.buffer))
		}

		requireReaderPayloadFailure(t, d, err)
	}
	for _, length := range []byte{1, 2, 3} {
		for _, isString := range []bool{true, false} {
			var wire []byte
			if isString {
				wire = []byte{0xd9, length}
			} else {
				wire = []byte{0xc4, length}
			}

			wire = append(wire, bytes.Repeat([]byte{0x78}, int(length))...)
			r := &decoderTestReader{data: wire}
			d := NewReader(r)
			if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: 2}); err != nil {
				t.Fatal(err)
			}

			var err error
			if isString {
				_, err = d.DecodeString()
			} else {
				_, err = d.DecodeBytes()
			}

			if length > 2 {
				if err != ErrLimitExceeded || r.position != 2 {
					t.Fatalf("above cap = %v, consumed %d", err, r.position)
				}

				continue
			}

			if err != nil || r.position != len(wire) {
				t.Fatalf("within cap = %v, consumed %d", err, r.position)
			}
		}
	}
}

func TestReaderOwnedPayloadIntOverflow(t *testing.T) {
	if strconv.IntSize != 32 {
		t.Skip("uint32 payload lengths fit int on this platform; 32-bit execution required")
	}

	for _, tag := range []byte{0xdb, 0xc6} {
		r := &decoderTestReader{data: []byte{tag, 0x80, 0, 0, 0, 0x78}}
		d := NewReader(r)
		if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: math.MaxUint32}); err != nil {
			t.Fatal(err)
		}

		var err error
		if tag == 0xdb {
			_, err = d.DecodeString()
		} else {
			_, err = d.DecodeBytes()
		}

		if err != ErrOverflow || r.position != 5 {
			t.Fatalf("int overflow = %v, consumed %d", err, r.position)
		}
	}
}

func TestReaderPayloadOwnership(t *testing.T) {
	var (
		wire = []byte{0xa3, 'a', 'b', 'c', 0xc4, 3, 1, 2, 3, 0xa1, 'z', 0xc4, 1, 9}
		d    = NewReader(bytes.NewReader(wire))
	)

	text, textErr := d.DecodeString()
	value, bytesErr := d.DecodeBytes()
	if textErr != nil || bytesErr != nil || text != "abc" || !bytes.Equal(value, []byte{1, 2, 3}) {
		t.Fatalf("initial owned values = (%q, %x, %v, %v)", text, value, textErr, bytesErr)
	}

	wire[1], wire[6] = 'x', 8
	if _, err := d.DecodeString(); err != nil {
		t.Fatal(err)
	}

	var (
		later []byte
		err   error
	)

	if later, err = d.DecodeBytes(); err != nil {
		t.Fatal(err)
	}

	later[0] = 7
	d.Reset(bytes.NewReader([]byte{0xc4, 3, 4, 5, 6}))
	if n, transferErr := d.DecodeBytesTo(io.Discard); n != 3 || transferErr != nil {
		t.Fatalf("scratch reuse transfer = (%d, %v)", n, transferErr)
	}

	if text != "abc" || !bytes.Equal(value, []byte{1, 2, 3}) {
		t.Fatalf("owned values changed after source/result/scratch/reset reuse: %q, %x", text, value)
	}
}

func TestReaderShortStringOwnership(t *testing.T) {
	for _, length := range []int{1, 8, 9, 10} {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			var (
				want  = strings.Repeat("\xff", length)
				wire  = append([]byte{0xa0 | byte(length)}, []byte(want)...)
				input bytes.Reader
				r     = NewReader(&input)
				value string
				err   error
			)

			input.Reset(wire)
			if value, err = r.DecodeString(); err != nil || value != want {
				t.Fatalf("DecodeString = (%q, %v), want %q", value, err, want)
			}

			wire[1] = 0
			input.Reset([]byte{0xa9, 1, 2, 3, 4, 5, 6, 7, 8, 9})
			r.Reset(&input)
			if _, err = r.DecodeString(); err != nil {
				t.Fatal(err)
			}

			if value != want {
				t.Fatalf("owned string changed after source mutation and scratch reuse: %q", value)
			}
		})
	}
}

func TestReaderShortStringAllocations(t *testing.T) {
	for _, length := range []int{0, 1, 2, 8, 9} {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			var (
				wire  = append([]byte{0xa0 | byte(length)}, bytes.Repeat([]byte{0xff}, length)...)
				input bytes.Reader
				r     = NewReader(&input)
			)

			allocations := testing.AllocsPerRun(1000, func() {
				input.Reset(wire)
				r.Reset(&input)
				var err error
				if benchmarkStringSink, err = r.DecodeString(); err != nil {
					t.Fatal(err)
				}
			})
			if allocations > 1 {
				t.Fatalf("short string decoding allocated %g times, want at most one owned string allocation", allocations)
			}
		})
	}
}

func TestReaderPayloadClassification(t *testing.T) {
	methods := []struct {
		name  string
		run   func(d *Reader) (err error)
		wrong byte
	}{
		{name: "string", wrong: 0xc6, run: func(d *Reader) (err error) {
			value, readErr := d.DecodeString()
			if value != "" {
				return fmt.Errorf("failed string result %q is not zero", value)
			}

			return readErr
		}},
		{name: "binary", wrong: 0xdb, run: func(d *Reader) (err error) {
			value, readErr := d.DecodeBytes()
			if value != nil {
				return fmt.Errorf("failed binary result %x is not nil", value)
			}

			return readErr
		}},
		{name: "string to", wrong: 0xc6, run: func(d *Reader) (err error) {
			n, readErr := d.DecodeStringTo(io.Discard)
			if n != 0 {
				return fmt.Errorf("failed string progress %d is not zero", n)
			}

			return readErr
		}},
		{name: "binary to", wrong: 0xdb, run: func(d *Reader) (err error) {
			n, readErr := d.DecodeBytesTo(io.Discard)
			if n != 0 {
				return fmt.Errorf("failed binary progress %d is not zero", n)
			}

			return readErr
		}},
	}
	for _, method := range methods {
		for _, tag := range []byte{0xc1, 0xc7, 0xc8, 0xc9, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xc0, 0xc3, 0x01, 0xff, 0xcf, 0xcb, 0x90, 0xdd, 0x80, 0xdf, method.wrong} {
			t.Run(fmt.Sprintf("%s/tag_%x", method.name, tag), func(t *testing.T) {
				var (
					r    = &decoderTestReader{data: []byte{tag, 0xc3}}
					d    = NewReader(r)
					want = readTypeError(tag)
					err  = method.run(d)
				)

				if err != want || r.position != 1 {
					t.Fatalf("classification = %v, consumed %d, want %v and one byte", err, r.position, want)
				}

				reads := r.reads
				requireReaderPayloadFailure(t, d, err)
				if r.reads != reads {
					t.Fatal("classification failure performed more I/O")
				}
			})
		}
	}
}

func TestReaderPayloadTransferValidationAndEmpty(t *testing.T) {
	methods := []struct {
		name  string
		empty []byte
		wire  []byte
		run   func(d *Reader, dst io.Writer) (n uint64, err error)
	}{
		{name: "string", empty: []byte{0xa0}, wire: []byte{0xa2, 1, 2}, run: (*Reader).DecodeStringTo},
		{name: "binary", empty: []byte{0xc4, 0}, wire: []byte{0xc4, 2, 1, 2}, run: (*Reader).DecodeBytesTo},
	}
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			var (
				unattached Reader
				r          = &decoderTestReader{data: append(append([]byte(nil), method.empty...), 0xc3)}
				d          = NewReader(r)
				writer     = &encoderPayloadWriter{failCall: 1, err: io.ErrClosedPipe}
			)

			if n, err := method.run(&unattached, nil); n != 0 || err != ErrUninitialized {
				t.Fatalf("unattached nil transfer = (%d, %v)", n, err)
			}

			if n, err := method.run(d, nil); n != 0 || err != ErrInvalidArgument || r.reads != 0 || d.state.err != nil {
				t.Fatalf("nil destination = (%d, %v), reads %d", n, err, r.reads)
			}

			if n, err := method.run(d, writer); n != 0 || err != nil || writer.writes != 0 || d.state.buffer != nil || r.position != len(method.empty) {
				t.Fatalf("empty transfer = (%d, %v), writes %d, consumed %d", n, err, writer.writes, r.position)
			}

			if next, err := d.DecodeBool(); !next || err != nil {
				t.Fatalf("following value after empty = (%v, %v)", next, err)
			}

			d.Reset(bytes.NewReader(method.wire))
			if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: 1}); err != nil {
				t.Fatal(err)
			}

			var typedNil *encoderTestWriter
			if n, err := method.run(d, typedNil); n != 2 || err != nil {
				t.Fatalf("valid typed-nil writer/independent cap = (%d, %v)", n, err)
			}
		})
	}
}

func TestReaderPayloadSourceErrors(t *testing.T) {
	var (
		failure = errors.New("test payload source failure")
		tests   = []struct {
			name  string
			tail  []byte
			end   error
			want  error
			wantN uint64
		}{
			{name: "no payload error", end: failure, want: failure},
			{name: "partial payload error", tail: []byte{1, 2}, end: failure, want: failure, wantN: 2},
			{name: "full payload error", tail: []byte{1, 2, 3, 4}, end: failure, want: failure, wantN: 4},
			{name: "no payload EOF", end: io.EOF, want: io.ErrUnexpectedEOF},
			{name: "partial payload EOF", tail: []byte{1, 2}, end: io.EOF, want: io.ErrUnexpectedEOF, wantN: 2},
			{name: "full payload EOF", tail: []byte{1, 2, 3, 4}, end: io.EOF, wantN: 4},
		}
	)

	for _, isString := range []bool{false, true} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("string_%t/%s", isString, tt.name), func(t *testing.T) {
				var header []byte
				if isString {
					header = []byte{0xd9, 4}
				} else {
					header = []byte{0xc4, 4}
				}

				var (
					wire = append(append([]byte(nil), header...), tt.tail...)
					r    = &decoderTestReader{data: wire, endErr: tt.end}
					d    = NewReader(r)
					err  error
				)

				if isString {
					value, readErr := d.DecodeString()
					err = readErr
					if tt.want != nil && value != "" {
						t.Fatalf("failed owned string = %q", value)
					}
				} else {
					value, readErr := d.DecodeBytes()
					err = readErr
					if tt.want != nil && value != nil {
						t.Fatalf("failed owned binary = %x", value)
					}
				}

				if !errors.Is(err, tt.want) || errors.Is(err, errFuzzTruncated) {
					t.Fatalf("owned source error = %v, want %v", err, tt.want)
				}

				r = &decoderTestReader{data: wire, endErr: tt.end}
				d.Reset(r)
				var (
					dst bytes.Buffer
					n   uint64
				)

				if isString {
					n, err = d.DecodeStringTo(&dst)
				} else {
					n, err = d.DecodeBytesTo(&dst)
				}

				if !errors.Is(err, tt.want) || n != tt.wantN || !bytes.Equal(dst.Bytes(), tt.tail) {
					t.Fatalf("source transfer = (%d, %v), payload %x", n, err, dst.Bytes())
				}

				if tt.want != nil {
					if err == tt.want || !strings.Contains(err.Error(), "source") || !strings.Contains(err.Error(), "decode") {
						t.Fatalf("source error lacks context: %v", err)
					}

					reads := r.reads
					requireReaderPayloadFailure(t, d, err)
					if reads != r.reads {
						t.Fatal("source failure performed more I/O")
					}
				}
			})
		}
	}
}

func TestReaderPayloadHeaderErrors(t *testing.T) {
	failure := errors.New("test payload header failure")
	for _, wire := range [][]byte{{0xa0}, {0xd9, 0}, {0xda, 0, 0}, {0xdb, 0, 0, 0, 0}, {0xc4, 0}, {0xc5, 0, 0}, {0xc6, 0, 0, 0, 0}} {
		r := &decoderTestReader{data: wire, endErr: failure}
		d := NewReader(r)
		var (
			n   uint64
			err error
		)

		if wire[0] >= 0xa0 && wire[0] <= 0xbf || wire[0] >= 0xd9 && wire[0] <= 0xdb {
			n, err = d.DecodeStringTo(io.Discard)
		} else {
			n, err = d.DecodeBytesTo(io.Discard)
		}

		if n != 0 || !errors.Is(err, failure) {
			t.Fatalf("full header plus error for %x = (%d, %v)", wire, n, err)
		}
	}
}

func TestReaderPayloadDestinationFailures(t *testing.T) {
	var (
		sourceFailure      = errors.New("test simultaneous source failure")
		destinationFailure = errors.New("test payload destination failure")
		tests              = []struct {
			name      string
			accept    int
			acceptAll bool
			writeErr  error
			readErr   error
			want      error
			wantN     uint64
		}{
			{name: "before output", writeErr: destinationFailure, want: destinationFailure},
			{name: "partial error", accept: 2, writeErr: destinationFailure, want: destinationFailure, wantN: 2},
			{name: "full error", acceptAll: true, writeErr: destinationFailure, want: destinationFailure, wantN: 4},
			{name: "short nil", accept: 2, want: io.ErrShortWrite, wantN: 2},
			{name: "zero progress", want: io.ErrShortWrite},
			{name: "simultaneous partial", accept: 2, writeErr: destinationFailure, readErr: sourceFailure, want: destinationFailure, wantN: 2},
			{name: "simultaneous full", acceptAll: true, writeErr: destinationFailure, readErr: sourceFailure, want: destinationFailure, wantN: 4},
			{name: "simultaneous short nil", accept: 2, readErr: sourceFailure, want: io.ErrShortWrite, wantN: 2},
		}
	)

	for _, method := range []struct {
		name   string
		header []byte
		run    func(d *Reader, dst io.Writer) (n uint64, err error)
	}{
		{name: "string", header: []byte{0xa4}, run: (*Reader).DecodeStringTo},
		{name: "binary", header: []byte{0xc4, 4}, run: (*Reader).DecodeBytesTo},
	} {
		for _, tt := range tests {
			t.Run(method.name+"/"+tt.name, func(t *testing.T) {
				var (
					r      = &decoderTestReader{data: append(append([]byte(nil), method.header...), 1, 2, 3, 4), endErr: tt.readErr}
					d      = NewReader(r)
					writer = &encoderPayloadWriter{failCall: 1, accept: tt.accept, acceptAll: tt.acceptAll, err: tt.writeErr}
					n, err = method.run(d, writer)
				)

				if n != tt.wantN || !errors.Is(err, tt.want) || errors.Is(err, sourceFailure) || !bytes.Equal(writer.data, []byte{1, 2, 3, 4}[:int(tt.wantN)]) {
					t.Fatalf("destination failure = (%d, %v), accepted %x", n, err, writer.data)
				}

				if err == tt.want || !strings.Contains(err.Error(), "destination") || !strings.Contains(err.Error(), "decode") {
					t.Fatalf("destination error lacks context: %v", err)
				}

				reads := r.reads
				requireReaderPayloadFailure(t, d, err)
				if r.reads != reads || writer.writes != 1 || writer.flushes != 0 || writer.closes != 0 {
					t.Fatal("destination failure retried/drained input or changed writer ownership")
				}
			})
		}
	}
}

func TestReaderPayloadZeroProgress(t *testing.T) {
	for _, empty := range []int{99, 100} {
		for _, isString := range []bool{false, true} {
			var header []byte
			if isString {
				header = []byte{0xa2}
			} else {
				header = []byte{0xc4, 2}
			}

			var (
				source = &decoderTestReader{data: []byte{1, 2}, maxRead: 1, initialEmpty: empty, emptyBetween: empty}
				d      = NewReader(io.MultiReader(bytes.NewReader(header), source))
				dst    bytes.Buffer
				n      uint64
				err    error
			)

			if isString {
				n, err = d.DecodeStringTo(&dst)
			} else {
				n, err = d.DecodeBytesTo(&dst)
			}

			if empty == 99 {
				if n != 2 || err != nil || !bytes.Equal(dst.Bytes(), []byte{1, 2}) {
					t.Fatalf("temporary empty reads = (%d, %v), bytes %x", n, err, dst.Bytes())
				}

				continue
			}

			if n != 0 || !errors.Is(err, io.ErrNoProgress) || source.reads != 100 || dst.Len() != 0 {
				t.Fatalf("zero progress = (%d, %v), reads %d", n, err, source.reads)
			}

			requireReaderPayloadFailure(t, d, err)
		}
	}
}

func TestReaderGeneratedPayloadTransfer(t *testing.T) {
	for _, method := range []struct {
		name   string
		header []byte
		run    func(d *Reader, dst io.Writer) (n uint64, err error)
	}{
		{name: "string", header: []byte{0xdb, 0, 0x80, 0, 7}, run: (*Reader).DecodeStringTo},
		{name: "binary", header: []byte{0xc6, 0, 0x80, 0, 7}, run: (*Reader).DecodeBytesTo},
	} {
		t.Run(method.name, func(t *testing.T) {
			const length = 8*1024*1024 + 7
			var (
				source = &encoderGeneratedReader{length: length}
				reader = io.MultiReader(bytes.NewReader(method.header), source, bytes.NewReader([]byte{0xc3}))
				d      = NewReader(reader)
				writer = &decoderCountingWriter{source: source}
			)

			if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: 1}); err != nil {
				t.Fatal(err)
			}

			n, err := method.run(d, writer)
			if err != nil || n != length || source.read != length || writer.bytes != length || writer.invalid {
				t.Fatalf("generated transfer = (%d, %v), source %d, sink %d, invalid %v", n, err, source.read, writer.bytes, writer.invalid)
			}

			if source.maxRequest > 32768 || writer.maxRequest > 32768 || writer.firstPayloadSourceRead > 32768 || writer.firstPayloadSourceRead >= length || len(d.state.buffer) > 32768 {
				t.Fatalf("transfer was not bounded/incremental: source request %d, sink request %d, first output after %d", source.maxRequest, writer.maxRequest, writer.firstPayloadSourceRead)
			}

			if next, nextErr := d.DecodeBool(); !next || nextErr != nil {
				t.Fatalf("following generated-payload value = (%v, %v)", next, nextErr)
			}
		})
	}
}

func TestReaderPayloadFailureDoesNotDrain(t *testing.T) {
	for _, failCall := range []int{1, 2} {
		var (
			source   = &encoderGeneratedReader{length: 8 * 1024 * 1024}
			d        = NewReader(io.MultiReader(bytes.NewReader([]byte{0xc6, 0, 0x80, 0, 0}), source))
			writer   = &encoderPayloadWriter{failCall: failCall, accept: 3, err: io.ErrClosedPipe}
			n, err   = d.DecodeBytesTo(writer)
			wantN    = uint64(failCall-1)*32768 + 3
			wantRead = uint64(failCall) * 32768
		)

		if n != wantN || !errors.Is(err, io.ErrClosedPipe) || source.read != wantRead || source.read == source.length {
			t.Fatalf("failed chunk %d = (%d, %v), source read %d", failCall, n, err, source.read)
		}

		reads := source.reads
		requireReaderPayloadFailure(t, d, err)
		if source.reads != reads || writer.writes != failCall {
			t.Fatal("failed generated payload retried or drained")
		}
	}
}

func TestReaderPayloadLaterChunkSourceFailure(t *testing.T) {
	failure := errors.New("test later payload source failure")
	for _, tag := range []byte{0xdb, 0xc6} {
		var (
			wire = append([]byte{tag, 0, 1, 0, 0}, bytes.Repeat([]byte{0x78}, 32771)...)
			r    = &decoderTestReader{data: wire, endErr: failure}
			d    = NewReader(r)
			dst  bytes.Buffer
			n    uint64
			err  error
		)

		if tag == 0xdb {
			n, err = d.DecodeStringTo(&dst)
		} else {
			n, err = d.DecodeBytesTo(&dst)
		}

		if n != 32771 || !errors.Is(err, failure) || dst.Len() != 32771 || r.position != len(wire) {
			t.Fatalf("later source transfer = (%d, %v), output %d, consumed %d", n, err, dst.Len(), r.position)
		}

		d.Reset(&decoderTestReader{data: wire, endErr: failure})
		if tag == 0xdb {
			value, readErr := d.DecodeString()
			if value != "" || !errors.Is(readErr, failure) {
				t.Fatalf("later owned string failure = (%q, %v)", value, readErr)
			}

			continue
		}

		value, readErr := d.DecodeBytes()
		if value != nil || !errors.Is(readErr, failure) {
			t.Fatalf("later owned binary failure = (%x, %v)", value, readErr)
		}
	}
}

func TestReaderPayloadNoProgressRetainsPartialCount(t *testing.T) {
	var (
		source = &decoderTestReader{data: []byte{1, 2}, maxRead: 1, emptyBetween: 100}
		d      = NewReader(io.MultiReader(bytes.NewReader([]byte{0xc4, 2}), source))
		dst    bytes.Buffer
		n, err = d.DecodeBytesTo(&dst)
	)

	if n != 1 || !errors.Is(err, io.ErrNoProgress) || !bytes.Equal(dst.Bytes(), []byte{1}) || source.reads != 101 {
		t.Fatalf("partial no progress = (%d, %v), output %x, reads %d", n, err, dst.Bytes(), source.reads)
	}
}

func TestReaderHugeTransferHeaderDoesNotMaterialize(t *testing.T) {
	for _, tag := range []byte{0xdb, 0xc6} {
		r := &decoderTestReader{data: []byte{tag, 0xff, 0xff, 0xff, 0xff, 1, 2}}
		d := NewReader(r)
		var (
			n   uint64
			err error
			dst bytes.Buffer
		)

		if tag == 0xdb {
			n, err = d.DecodeStringTo(&dst)
		} else {
			n, err = d.DecodeBytesTo(&dst)
		}

		if n != 2 || !errors.Is(err, io.ErrUnexpectedEOF) || !bytes.Equal(dst.Bytes(), []byte{1, 2}) || len(d.state.buffer) != 32768 {
			t.Fatalf("huge transfer header = (%d, %v), payload %x, scratch %d", n, err, dst.Bytes(), len(d.state.buffer))
		}
	}
}

func TestSkipSupportedValues(t *testing.T) {
	tests := []struct {
		name string
		wire []byte
	}{
		{name: "positive fixint", wire: []byte{0x7f}},
		{name: "negative fixint", wire: []byte{0xe0}},
		{name: "nil", wire: []byte{0xc0}},
		{name: "false", wire: []byte{0xc2}},
		{name: "true", wire: []byte{0xc3}},
		{name: "uint8", wire: []byte{0xcc, 0xff}},
		{name: "uint16", wire: []byte{0xcd, 0xff, 0xff}},
		{name: "uint32", wire: []byte{0xce, 0xff, 0xff, 0xff, 0xff}},
		{name: "uint64 without signed conversion", wire: []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "int8", wire: []byte{0xd0, 0x80}},
		{name: "int16", wire: []byte{0xd1, 0x80, 0}},
		{name: "int32", wire: []byte{0xd2, 0x80, 0, 0, 0}},
		{name: "int64", wire: []byte{0xd3, 0x80, 0, 0, 0, 0, 0, 0, 0}},
		{name: "float32 NaN bits", wire: []byte{0xca, 0x7f, 0x81, 0x23, 0x45}},
		{name: "float64 NaN bits", wire: []byte{0xcb, 0x7f, 0xf0, 0, 0, 0, 0, 0, 1}},
		{name: "empty fixstr", wire: []byte{0xa0}},
		{name: "raw fixstr", wire: []byte{0xa2, 0xff, 0}},
		{name: "nine byte string", wire: []byte{0xa9, 0xff, 0, 1, 2, 3, 4, 5, 6, 7}},
		{name: "ten byte string", wire: []byte{0xaa, 0xff, 0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{name: "str8", wire: []byte{0xd9, 2, 0xff, 0}},
		{name: "str16", wire: []byte{0xda, 0, 2, 0xff, 0}},
		{name: "str32", wire: []byte{0xdb, 0, 0, 0, 2, 0xff, 0}},
		{name: "empty str8", wire: []byte{0xd9, 0}},
		{name: "empty str16", wire: []byte{0xda, 0, 0}},
		{name: "empty str32", wire: []byte{0xdb, 0, 0, 0, 0}},
		{name: "bin8", wire: []byte{0xc4, 2, 0xff, 0}},
		{name: "nine byte binary", wire: []byte{0xc4, 9, 0xff, 0, 1, 2, 3, 4, 5, 6, 7}},
		{name: "ten byte binary", wire: []byte{0xc4, 10, 0xff, 0, 1, 2, 3, 4, 5, 6, 7, 8}},
		{name: "bin16", wire: []byte{0xc5, 0, 2, 0xff, 0}},
		{name: "bin32", wire: []byte{0xc6, 0, 0, 0, 2, 0xff, 0}},
		{name: "empty bin8", wire: []byte{0xc4, 0}},
		{name: "empty bin16", wire: []byte{0xc5, 0, 0}},
		{name: "empty bin32", wire: []byte{0xc6, 0, 0, 0, 0}},
		{name: "empty fixarray", wire: []byte{0x90}},
		{name: "array16", wire: []byte{0xdc, 0, 2, 0xc0, 0xc3}},
		{name: "array32", wire: []byte{0xdd, 0, 0, 0, 2, 0xc0, 0xc3}},
		{name: "empty array16", wire: []byte{0xdc, 0, 0}},
		{name: "empty array32", wire: []byte{0xdd, 0, 0, 0, 0}},
		{name: "empty fixmap", wire: []byte{0x80}},
		{name: "map16", wire: []byte{0xde, 0, 1, 0xc0, 0xc3}},
		{name: "map32", wire: []byte{0xdf, 0, 0, 0, 1, 0xc0, 0xc3}},
		{name: "empty map16", wire: []byte{0xde, 0, 0}},
		{name: "empty map32", wire: []byte{0xdf, 0, 0, 0, 0}},
		{name: "nested last child containers", wire: []byte{0x92, 0x91, 0x81, 0xc0, 0x90, 0x81, 0xc0, 0x91, 0xc3}},
		{name: "containers as map keys", wire: []byte{0x81, 0x91, 1, 0x81, 0xc0, 0xc3}},
	}
	for _, tt := range tests {
		for _, fragment := range []int{0, 1, 3} {
			t.Run(fmt.Sprintf("%s/fragment_%d", tt.name, fragment), func(t *testing.T) {
				var (
					wire = append(append([]byte(nil), tt.wire...), 0xc3)
					r    = &decoderTestReader{data: wire, maxRead: fragment, endErr: io.EOF}
					d    = NewReader(r)
				)

				if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: 1}); err != nil {
					t.Fatal(err)
				}

				if err := d.Skip(); err != nil || r.position != len(tt.wire) {
					t.Fatalf("Skip() = %v, position %d, want %d", err, r.position, len(tt.wire))
				}

				if value, err := d.DecodeBool(); err != nil || !value {
					t.Fatalf("value after Skip() = (%v, %v)", value, err)
				}

				if r.closes != 0 {
					t.Fatal("Skip closed its source")
				}
			})
		}
	}
}

func TestSkipShortPayloadAllocations(t *testing.T) {
	for _, wire := range [][]byte{
		{0xa0},
		{0xa1, 'x'},
		{0xa9, '1', '2', '3', '4', '5', '6', '7', '8', '9'},
		{0xc4, 0},
		{0xc4, 1, 'x'},
		{0xc4, 9, '1', '2', '3', '4', '5', '6', '7', '8', '9'},
	} {
		t.Run(fmt.Sprintf("%x", wire), func(t *testing.T) {
			var input bytes.Reader
			allocations := testing.AllocsPerRun(1000, func() {
				input.Reset(wire)
				var r = NewReader(&input)
				if err := r.Skip(); err != nil {
					t.Fatal(err)
				}
			})
			if allocations > 2 {
				t.Fatalf("fresh short payload Skip allocated %g times, want at most two codec allocations", allocations)
			}
		})
	}
}

func TestSkipGeneratedPayloadBoundaries(t *testing.T) {
	tests := []struct {
		name   string
		header []byte
		length uint64
	}{
		{name: "fixstr maximum", header: []byte{0xbf}, length: 31},
		{name: "str8 minimum", header: []byte{0xd9, 32}, length: 32},
		{name: "str8 maximum", header: []byte{0xd9, 0xff}, length: 255},
		{name: "str16 minimum", header: []byte{0xda, 1, 0}, length: 256},
		{name: "str16 maximum", header: []byte{0xda, 0xff, 0xff}, length: 65535},
		{name: "str32 minimum", header: []byte{0xdb, 0, 1, 0, 0}, length: 65536},
		{name: "bin8 maximum", header: []byte{0xc4, 0xff}, length: 255},
		{name: "bin16 minimum", header: []byte{0xc5, 1, 0}, length: 256},
		{name: "bin16 maximum", header: []byte{0xc5, 0xff, 0xff}, length: 65535},
		{name: "bin32 minimum", header: []byte{0xc6, 0, 1, 0, 0}, length: 65536},
		{name: "large string", header: []byte{0xdb, 0, 0x80, 0, 7}, length: 8*1024*1024 + 7},
		{name: "large binary", header: []byte{0xc6, 0, 0x80, 0, 7}, length: 8*1024*1024 + 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				payload = &encoderGeneratedReader{length: tt.length}
				next    = bytes.NewReader([]byte{0xc3})
				d       = NewReader(io.MultiReader(bytes.NewReader(tt.header), payload, next))
			)

			if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: 1, MaxSkipBytes: uint64(len(tt.header)) + tt.length}); err != nil {
				t.Fatal(err)
			}

			if err := d.Skip(); err != nil || payload.read != tt.length || next.Len() != 1 {
				t.Fatalf("Skip() = %v, generated %d bytes, next bytes %d", err, payload.read, next.Len())
			}

			if payload.maxRequest > payloadBufferSize || len(d.state.buffer) != payloadBufferSize {
				t.Fatalf("payload request %d, scratch %d", payload.maxRequest, len(d.state.buffer))
			}

			if value, err := d.DecodeBool(); err != nil || !value {
				t.Fatalf("following value = (%v, %v)", value, err)
			}
		})
	}
}

func TestSkipGeneratedContainerBoundaries(t *testing.T) {
	tests := []struct {
		name     string
		header   []byte
		children uint64
	}{
		{name: "fixarray maximum", header: []byte{0x9f}, children: 15},
		{name: "array16 minimum", header: []byte{0xdc, 0, 16}, children: 16},
		{name: "array16 maximum", header: []byte{0xdc, 0xff, 0xff}, children: 65535},
		{name: "array32 minimum", header: []byte{0xdd, 0, 1, 0, 0}, children: 65536},
		{name: "fixmap maximum", header: []byte{0x8f}, children: 30},
		{name: "map16 minimum", header: []byte{0xde, 0, 16}, children: 32},
		{name: "map16 maximum", header: []byte{0xde, 0xff, 0xff}, children: 131070},
		{name: "map32 minimum", header: []byte{0xdf, 0, 1, 0, 0}, children: 131072},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				children = &skipRepeatingReader{value: 0xc0, remaining: tt.children}
				next     = bytes.NewReader([]byte{0xc3})
				d        = NewReader(io.MultiReader(bytes.NewReader(tt.header), children, next))
			)

			if err := d.SetLimits(ReaderLimits{MaxSkipValues: tt.children + 1, MaxSkipBytes: uint64(len(tt.header)) + tt.children}); err != nil {
				t.Fatal(err)
			}

			if err := d.Skip(); err != nil || children.read != tt.children || next.Len() != 1 {
				t.Fatalf("Skip() = %v, children %d, next bytes %d", err, children.read, next.Len())
			}

			if children.maxRequest != 1 || d.state.buffer != nil {
				t.Fatal("Skip requested or buffered the container as a whole")
			}

			if value, err := d.DecodeBool(); err != nil || !value {
				t.Fatalf("following value = (%v, %v)", value, err)
			}
		})
	}
}

func TestSkipTruncation(t *testing.T) {
	wires := [][]byte{
		{0xcc, 0xff}, {0xcd, 0xff, 0xff}, {0xce, 0xff, 0xff, 0xff, 0xff},
		{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		{0xd0, 0x80}, {0xd1, 0x80, 0}, {0xd2, 0x80, 0, 0, 0},
		{0xd3, 0x80, 0, 0, 0, 0, 0, 0, 0},
		{0xca, 0x3f, 0xc0, 0, 0}, {0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0},
		{0xa2, 0xff, 0}, {0xd9, 2, 0xff, 0}, {0xda, 0, 2, 0xff, 0}, {0xdb, 0, 0, 0, 2, 0xff, 0},
		{0xa9, 0xff, 0, 1, 2, 3, 4, 5, 6, 7}, {0xaa, 0xff, 0, 1, 2, 3, 4, 5, 6, 7, 8},
		{0xc4, 2, 0xff, 0}, {0xc5, 0, 2, 0xff, 0}, {0xc6, 0, 0, 0, 2, 0xff, 0},
		{0xc4, 9, 0xff, 0, 1, 2, 3, 4, 5, 6, 7}, {0xc4, 10, 0xff, 0, 1, 2, 3, 4, 5, 6, 7, 8},
		{0x91, 0xc0}, {0xdc, 0, 1, 0xc0}, {0xdd, 0, 0, 0, 1, 0xc0},
		{0x81, 0xc0, 0xc3}, {0xde, 0, 1, 0xc0, 0xc3}, {0xdf, 0, 0, 0, 1, 0xc0, 0xc3},
		{0x92, 0x81, 0xc0, 0x91, 0xc3, 0x90},
	}
	for _, wire := range wires {
		for length := 0; length < len(wire); length++ {
			t.Run(fmt.Sprintf("%x/length_%d", wire, length), func(t *testing.T) {
				var (
					r    = &decoderTestReader{data: wire[:length], maxRead: 1, endErr: io.EOF}
					d    = NewReader(r)
					want = io.ErrUnexpectedEOF
					err  = d.Skip()
				)

				if length == 0 {
					want = io.EOF
				}

				if !errors.Is(err, want) || errors.Is(err, errFuzzTruncated) || err == want || r.position != length {
					t.Fatalf("Skip() = %v, consumed %d, want wrapped %v after %d", err, r.position, want, length)
				}

				if length > 0 && errors.Is(err, io.EOF) {
					t.Fatal("incomplete root also matched clean EOF")
				}

				reads := r.reads
				requireSkipFailure(t, d, err)
				if r.reads != reads {
					t.Fatal("terminal Skip failure performed more I/O")
				}
			})
		}
	}
}

func TestSkipLimits(t *testing.T) {
	tests := []struct {
		name     string
		wire     []byte
		limits   ReaderLimits
		consumed int
		want     error
	}{
		{name: "scalar exact", wire: []byte{0xc0}, limits: ReaderLimits{MaxSkipValues: 1, MaxSkipBytes: 1}, consumed: 1},
		{name: "empty container exact", wire: []byte{0x90}, limits: ReaderLimits{MaxSkipDepth: 1, MaxSkipValues: 1, MaxSkipBytes: 1}, consumed: 1},
		{name: "array values exact", wire: []byte{0x91, 0xc0}, limits: ReaderLimits{MaxSkipValues: 2}, consumed: 2},
		{name: "array values over", wire: []byte{0x91, 0xc0}, limits: ReaderLimits{MaxSkipValues: 1}, consumed: 1, want: ErrLimitExceeded},
		{name: "map values exact", wire: []byte{0x81, 0xc0, 0xc3}, limits: ReaderLimits{MaxSkipValues: 3}, consumed: 3},
		{name: "map counts both key and value", wire: []byte{0x81, 0xc0, 0xc3}, limits: ReaderLimits{MaxSkipValues: 2}, consumed: 1, want: ErrLimitExceeded},
		{name: "ancestor sibling values exact", wire: []byte{0x92, 0x91, 0xc0, 0xc0}, limits: ReaderLimits{MaxSkipValues: 4}, consumed: 4},
		{name: "ancestor sibling values over", wire: []byte{0x92, 0x91, 0xc0, 0xc0}, limits: ReaderLimits{MaxSkipValues: 3}, consumed: 2, want: ErrLimitExceeded},
		{name: "ancestor map values over", wire: []byte{0x92, 0x81, 0xc0, 0xc0, 0xc0}, limits: ReaderLimits{MaxSkipValues: 4}, consumed: 2, want: ErrLimitExceeded},
		{name: "integer bytes exact", wire: []byte{0xcf, 0, 0, 0, 0, 0, 0, 0, 7}, limits: ReaderLimits{MaxSkipBytes: 9}, consumed: 9},
		{name: "integer bytes over", wire: []byte{0xcf, 0, 0, 0, 0, 0, 0, 0, 7}, limits: ReaderLimits{MaxSkipBytes: 8}, consumed: 1, want: ErrLimitExceeded},
		{name: "payload bytes exact", wire: []byte{0xdb, 0, 0, 0, 3, 0, 1, 2}, limits: ReaderLimits{MaxSkipBytes: 8}, consumed: 8},
		{name: "payload bytes over", wire: []byte{0xdb, 0, 0, 0, 3, 0, 1, 2}, limits: ReaderLimits{MaxSkipBytes: 7}, consumed: 5, want: ErrLimitExceeded},
		{name: "fixed payload bytes over", wire: []byte{0xa2, 0, 1}, limits: ReaderLimits{MaxSkipBytes: 2}, consumed: 1, want: ErrLimitExceeded},
		{name: "child tag bytes exact", wire: []byte{0x91, 0xc0}, limits: ReaderLimits{MaxSkipBytes: 2}, consumed: 2},
		{name: "child tag bytes over", wire: []byte{0x91, 0xc0}, limits: ReaderLimits{MaxSkipBytes: 1}, consumed: 1, want: ErrLimitExceeded},
		{name: "ancestor tag bytes over", wire: []byte{0x92, 0x92, 0xc0, 0xc0, 0xc0}, limits: ReaderLimits{MaxSkipBytes: 4}, consumed: 2, want: ErrLimitExceeded},
		{name: "header budget precedes missing header", wire: []byte{0xda}, limits: ReaderLimits{MaxSkipBytes: 2}, consumed: 1, want: ErrLimitExceeded},
		{name: "header budget permits detecting truncation", wire: []byte{0xda}, limits: ReaderLimits{MaxSkipBytes: 3}, consumed: 1, want: io.ErrUnexpectedEOF},
		{name: "body budget precedes missing body", wire: []byte{0xcb}, limits: ReaderLimits{MaxSkipBytes: 8}, consumed: 1, want: ErrLimitExceeded},
		{name: "work budget precedes missing child", wire: []byte{0x91}, limits: ReaderLimits{MaxSkipValues: 1}, consumed: 1, want: ErrLimitExceeded},
		{name: "work budget permits detecting missing child", wire: []byte{0x91}, limits: ReaderLimits{MaxSkipValues: 2}, consumed: 1, want: io.ErrUnexpectedEOF},
		{name: "maximum array default values", wire: []byte{0xdd, 0xff, 0xff, 0xff, 0xff}, consumed: 5, want: ErrLimitExceeded},
		{name: "maximum map default values", wire: []byte{0xdf, 0xff, 0xff, 0xff, 0xff}, consumed: 5, want: ErrLimitExceeded},
		{name: "maximum map uses wide child count", wire: []byte{0xdf, 0xff, 0xff, 0xff, 0xff}, limits: ReaderLimits{MaxSkipValues: 4294967296, MaxSkipBytes: math.MaxUint64}, consumed: 5, want: ErrLimitExceeded},
		{name: "maximum array minimum byte work", wire: []byte{0xdd, 0xff, 0xff, 0xff, 0xff}, limits: ReaderLimits{MaxSkipValues: math.MaxUint64, MaxSkipBytes: 5}, consumed: 5, want: ErrLimitExceeded},
		{name: "maximum string length budget", wire: []byte{0xdb, 0xff, 0xff, 0xff, 0xff}, consumed: 5, want: ErrLimitExceeded},
		{name: "maximum binary length budget", wire: []byte{0xc6, 0xff, 0xff, 0xff, 0xff}, consumed: 5, want: ErrLimitExceeded},
		{name: "maximum map maximal budgets", wire: []byte{0xdf, 0xff, 0xff, 0xff, 0xff}, limits: ReaderLimits{MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64}, consumed: 5, want: io.ErrUnexpectedEOF},
		{name: "maximum array maximal budgets", wire: []byte{0xdd, 0xff, 0xff, 0xff, 0xff}, limits: ReaderLimits{MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64}, consumed: 5, want: io.ErrUnexpectedEOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r = &decoderTestReader{data: tt.wire, maxRead: 1}
				d = NewReader(r)
			)

			if err := d.SetLimits(tt.limits); err != nil {
				t.Fatal(err)
			}

			err := d.Skip()
			if !errors.Is(err, tt.want) || r.position != tt.consumed {
				t.Fatalf("Skip() = %v, consumed %d, want %v after %d", err, r.position, tt.want, tt.consumed)
			}

			if tt.want == ErrLimitExceeded && err != tt.want {
				t.Fatal("codec limit sentinel was wrapped")
			}

			if err != nil {
				reads := r.reads
				requireSkipFailure(t, d, err)
				if r.reads != reads {
					t.Fatal("limit failure performed further I/O")
				}
			}
		})
	}
}

func TestSkipDepthLimits(t *testing.T) {
	for _, depth := range []uint32{1, 2, 64, 256} {
		for _, end := range []byte{0x90, 0x80, 0xc0} {
			t.Run(fmt.Sprintf("depth_%d/end_%x", depth, end), func(t *testing.T) {
				var (
					containers = depth
					wire       []byte
				)

				if end == 0x90 || end == 0x80 {
					containers--
				}

				wire = append(bytes.Repeat([]byte{0x91}, int(containers)), end)
				d := NewReader(bytes.NewReader(wire))
				if err := d.SetLimits(ReaderLimits{MaxSkipDepth: depth}); err != nil {
					t.Fatal(err)
				}

				if err := d.Skip(); err != nil {
					t.Fatalf("allowed depth failed: %v", err)
				}

				wire = append([]byte{0x91}, wire...)
				r := &decoderTestReader{data: wire, maxRead: 1}
				d.Reset(r)
				if err := d.Skip(); err != ErrLimitExceeded || r.position != int(depth)+1 {
					t.Fatalf("excess depth = %v, consumed %d, want limit after %d", err, r.position, depth+1)
				}
			})
		}
	}
}

func requireSkipFailure(t *testing.T, d *Reader, want error) {
	t.Helper()
	if err := d.Skip(); err != want {
		t.Fatalf("later Skip() = %v, want identical %v", err, want)
	}

	requireReaderScalarFailure(t, d, want)
	if n, err := d.DecodeBytesTo(nil); n != 0 || err != want {
		t.Fatalf("later invalid transfer = (%d, %v), want stored failure", n, err)
	}
}

func TestSkipClassificationAndSourceErrors(t *testing.T) {
	failure := errors.New("skip test source failure")
	tests := []struct {
		name string
		wire []byte
		end  error
		want error
	}{
		{name: "reserved", wire: []byte{0xc1}, want: ErrInvalidFormat},
		{name: "nested reserved", wire: []byte{0x91, 0xc1}, want: ErrInvalidFormat},
		{name: "no bytes source failure", end: failure, want: failure},
		{name: "complete tag source failure", wire: []byte{0xc0}, end: failure, want: failure},
		{name: "tag failure beats reserved", wire: []byte{0xc1}, end: failure, want: failure},
		{name: "partial scalar failure", wire: []byte{0xcf, 0, 0}, end: failure, want: failure},
		{name: "complete scalar failure", wire: []byte{0xcf, 0, 0, 0, 0, 0, 0, 0, 7}, end: failure, want: failure},
		{name: "partial payload failure", wire: []byte{0xc4, 3, 1}, end: failure, want: failure},
		{name: "complete payload failure", wire: []byte{0xc4, 3, 1, 2, 3}, end: failure, want: failure},
		{name: "last child failure", wire: []byte{0x91, 0xc0}, end: failure, want: failure},
		{name: "ext8", wire: []byte{0xc7}, want: ErrUnsupportedType},
		{name: "ext16", wire: []byte{0xc8}, want: ErrUnsupportedType},
		{name: "ext32", wire: []byte{0xc9}, want: ErrUnsupportedType},
		{name: "fixext1", wire: []byte{0xd4}, want: ErrUnsupportedType},
		{name: "fixext2", wire: []byte{0xd5}, want: ErrUnsupportedType},
		{name: "fixext4", wire: []byte{0xd6}, want: ErrUnsupportedType},
		{name: "fixext8", wire: []byte{0xd7}, want: ErrUnsupportedType},
		{name: "fixext16", wire: []byte{0xd8}, want: ErrUnsupportedType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r   = &decoderTestReader{data: tt.wire, endErr: tt.end}
				d   = NewReader(r)
				err = d.Skip()
			)

			if !errors.Is(err, tt.want) {
				t.Fatalf("Skip() = %v, want %v", err, tt.want)
			}

			if tt.want == failure && (err == failure || !strings.Contains(err.Error(), "skip source")) {
				t.Fatal("source error lacks operation context")
			}

			if tt.want != failure && err != tt.want {
				t.Fatal("codec sentinel was wrapped")
			}

			reads := r.reads
			requireSkipFailure(t, d, err)
			if r.reads != reads {
				t.Fatal("source/classification failure performed later reads")
			}
		})
	}
}

func TestSkipEOFAndLifecycle(t *testing.T) {
	var unattached Reader
	if err := unattached.Skip(); err != ErrUninitialized || unattached.state != nil {
		t.Fatalf("zero Skip() = %v, state %+v", err, unattached.state)
	}

	for _, wire := range [][]byte{{0xc0}, {0x91, 0xc0}, {0x81, 0xc0, 0x90}, {0xc4, 1, 0xff}} {
		r := &decoderTestReader{data: wire, maxRead: 1, endErr: io.EOF}
		d := NewReader(r)
		if err := d.Skip(); err != nil {
			t.Fatalf("complete data with EOF = %v", err)
		}

		reads := r.reads
		var err = d.Skip()
		if !errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || r.reads != reads {
			t.Fatalf("pending EOF = %v, reads %d, previous %d", err, r.reads, reads)
		}

		requireSkipFailure(t, d, err)
		d.Reset(nil)
		if err = d.Skip(); err != ErrUninitialized || r.closes != 0 || r.reads != reads {
			t.Fatalf("nil reset = %v, closes %d, reads %d", err, r.closes, r.reads)
		}

		d.Reset(bytes.NewReader([]byte{0x90}))
		if err = d.Skip(); err != nil {
			t.Fatalf("reset after EOF = %v", err)
		}
	}
	var typedNil *decoderTestReader
	if err := NewReader(typedNil).Skip(); err != nil {
		t.Fatalf("typed-nil reader's ordinary method failed: %v", err)
	}
}

func TestSkipBudgetsRestartAndReset(t *testing.T) {
	var (
		r = &decoderTestReader{data: []byte{0x91, 0xc0, 0x91, 0xc0, 0xc3}}
		d = NewReader(r)
	)

	if err := d.SetLimits(ReaderLimits{MaxSkipDepth: 1, MaxSkipValues: 2, MaxSkipBytes: 2}); err != nil {
		t.Fatal(err)
	}

	for index := 0; index < 2; index++ {
		if err := d.Skip(); err != nil || r.position != 2*(index+1) {
			t.Fatalf("Skip %d = %v, position %d", index, err, r.position)
		}
	}
	if value, err := d.DecodeBool(); err != nil || !value {
		t.Fatalf("following value = (%v, %v)", value, err)
	}

	d.Reset(bytes.NewReader([]byte{0x91, 0x90}))
	failure := d.Skip()
	if failure != ErrLimitExceeded {
		t.Fatalf("depth failure = %v", failure)
	}

	if err := d.SetLimits(ReaderLimits{MaxSkipDepth: 2}); err != nil {
		t.Fatal(err)
	}

	requireSkipFailure(t, d, failure)
	d.Reset(bytes.NewReader([]byte{0x91, 0x90}))
	if err := d.Skip(); err != nil || d.state.limits.MaxSkipDepth != 2 {
		t.Fatalf("reset with new limits = %v, depth %d", err, d.state.limits.MaxSkipDepth)
	}
}

func TestSkipZeroProgress(t *testing.T) {
	tests := []struct {
		name         string
		initialEmpty int
		emptyBetween int
		want         error
	}{
		{name: "temporary empty before tag", initialEmpty: 99},
		{name: "terminal empty before tag", initialEmpty: 100, want: io.ErrNoProgress},
		{name: "progress resets counter", emptyBetween: 99},
		{name: "terminal empty within value", emptyBetween: 100, want: io.ErrNoProgress},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r = &decoderTestReader{data: []byte{0xcd, 0, 1}, maxRead: 1,
					initialEmpty: tt.initialEmpty, emptyBetween: tt.emptyBetween}
				d   = NewReader(r)
				err = d.Skip()
			)

			if !errors.Is(err, tt.want) {
				t.Fatalf("Skip() = %v, want %v", err, tt.want)
			}

			if err != nil {
				reads := r.reads
				requireSkipFailure(t, d, err)
				if r.reads != reads {
					t.Fatal("zero-progress failure performed further reads")
				}
			}
		})
	}
}

func TestSkipLargePayloadPreflightAndFailures(t *testing.T) {
	const length uint64 = 32768 + 9
	var (
		failure = errors.New("late skip source failure")
		header  = []byte{0xc6, 0, 0, 0x80, 9}
		tests   = []struct {
			name       string
			byteBudget uint64
			available  uint64
			end        error
			want       error
			read       uint64
		}{
			{name: "declared payload exceeds budget", byteBudget: length + 4,
				available: length, want: ErrLimitExceeded},
			{name: "later chunk EOF", byteBudget: length + 5,
				available: length - 2, end: io.EOF, want: io.ErrUnexpectedEOF, read: length - 2},
			{name: "later chunk source error", byteBudget: length + 5,
				available: length - 2, end: failure, want: failure, read: length - 2},
		}
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				payload = &encoderGeneratedReader{length: tt.available}
				end     = &decoderTestReader{endErr: tt.end}
				d       = NewReader(io.MultiReader(bytes.NewReader(header), payload, end))
			)

			if err := d.SetLimits(ReaderLimits{MaxSkipBytes: tt.byteBudget}); err != nil {
				t.Fatal(err)
			}

			err := d.Skip()
			if !errors.Is(err, tt.want) || payload.read != tt.read || payload.maxRequest > payloadBufferSize {
				t.Fatalf("Skip() = %v, source bytes %d, max request %d", err, payload.read, payload.maxRequest)
			}

			if tt.want == ErrLimitExceeded && (d.state.buffer != nil || payload.reads != 0 || end.reads != 0) {
				t.Fatal("over-budget payload allocated scratch or accessed its source")
			}

			reads, endReads := payload.reads, end.reads
			requireSkipFailure(t, d, err)
			if payload.reads != reads || end.reads != endReads {
				t.Fatal("payload failure performed later reads")
			}
		})
	}
}

func TestStreamContainerHeaderOnlyAndWiderForms(t *testing.T) {
	tests := []struct {
		name  string
		wire  []byte
		isMap bool
		count uint32
	}{
		{name: "array16 empty", wire: []byte{0xdc, 0, 0}},
		{name: "array32 one", wire: []byte{0xdd, 0, 0, 0, 1}, count: 1},
		{name: "map16 empty", wire: []byte{0xde, 0, 0}, isMap: true},
		{name: "map32 one", wire: []byte{0xdf, 0, 0, 0, 1}, isMap: true, count: 1},
		{name: "array max no children", wire: []byte{0xdd, 0xff, 0xff, 0xff, 0xff}, count: math.MaxUint32},
		{name: "map max no children", wire: []byte{0xdf, 0xff, 0xff, 0xff, 0xff}, isMap: true, count: math.MaxUint32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r     = &decoderTestReader{data: tt.wire, endErr: io.EOF}
				d     = NewReader(r)
				count uint32
				err   error
			)

			if tt.isMap {
				count, err = d.DecodeMapHeader()
			} else {
				count, err = d.DecodeArrayHeader()
			}

			if err != nil || count != tt.count || r.position != len(tt.wire) || d.state.buffer != nil {
				t.Fatalf("header-only read = (%d, %v), consumed %d, buffer %d", count, err, r.position, len(d.state.buffer))
			}

			reads := r.reads
			if err = d.DecodeNil(); !errors.Is(err, io.EOF) || r.reads != reads {
				t.Fatalf("pending EOF = %v, reads %d, want %d", err, r.reads, reads)
			}
		})
	}
}

func TestStreamContainerTruncationAndClassification(t *testing.T) {
	for _, isMap := range []bool{false, true} {
		var wires = [][]byte{{0x90}, {0xdc, 0, 16}, {0xdd, 0, 1, 0, 0}}
		if isMap {
			wires = [][]byte{{0x80}, {0xde, 0, 16}, {0xdf, 0, 1, 0, 0}}
		}

		for _, wire := range wires {
			for length := 0; length < len(wire); length++ {
				t.Run(fmt.Sprintf("map_%t/tag_%x/length_%d", isMap, wire[0], length), func(t *testing.T) {
					var (
						r     = &decoderTestReader{data: wire[:length], maxRead: 1}
						d     = NewReader(r)
						count uint32
						err   error
						want  = io.ErrUnexpectedEOF
					)

					if length == 0 {
						want = io.EOF
					}

					if isMap {
						count, err = d.DecodeMapHeader()
					} else {
						count, err = d.DecodeArrayHeader()
					}

					if count != 0 || !errors.Is(err, want) || errors.Is(err, errFuzzTruncated) || r.position != length {
						t.Fatalf("truncated header = (%d, %v), position %d", count, err, r.position)
					}

					reads := r.reads
					requireStreamHeaderFailure(t, d, err)
					if r.reads != reads {
						t.Fatal("terminal header failure performed further reads")
					}
				})
			}
		}
		for _, tag := range []byte{0xc1, 0xc7, 0xc8, 0xc9, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xc0, 0xdb, 0x90, 0x80, 0xdd, 0xdf} {
			if isMap && (tag == 0x80 || tag == 0xdf) || !isMap && (tag == 0x90 || tag == 0xdd) {
				continue
			}

			var (
				r     = &decoderTestReader{data: []byte{tag, 0xc3}}
				d     = NewReader(r)
				count uint32
				err   error
				want  = readTypeError(tag)
			)

			if isMap {
				count, err = d.DecodeMapHeader()
			} else {
				count, err = d.DecodeArrayHeader()
			}

			if count != 0 || err != want || r.position != 1 {
				t.Fatalf("map %t rejected tag %x = (%d, %v), consumed %d, want %v", isMap, tag, count, err, r.position, want)
			}
		}
	}
}

func TestReaderPeekTagSharesPendingEOF(t *testing.T) {
	var (
		input = &decoderTestReader{data: []byte{0xc3}, endErr: io.EOF}
		r     = NewReader(input)
		tag   byte
		err   error
		value bool
	)

	if tag, err = r.peekTag(); err != nil || tag != 0xc3 || input.reads != 1 {
		t.Fatalf("peekTag = (%x, %v), reads %d", tag, err, input.reads)
	}

	var shared = *r
	if tag, err = shared.peekTag(); err != nil || tag != 0xc3 || input.reads != 1 {
		t.Fatalf("shared peekTag = (%x, %v), reads %d", tag, err, input.reads)
	}

	if value, err = shared.DecodeBool(); err != nil || !value || input.reads != 1 {
		t.Fatalf("shared DecodeBool = (%v, %v), reads %d", value, err, input.reads)
	}

	if _, err = r.DecodeBool(); !errors.Is(err, io.EOF) || input.reads != 1 {
		t.Fatalf("DecodeBool after cached EOF = %v, reads %d", err, input.reads)
	}
}

func TestReaderPeekTagSkipAndReset(t *testing.T) {
	var (
		input = &decoderTestReader{data: []byte{0x91, 42, 0xc2}}
		r     = NewReader(input)
		value bool
		err   error
	)

	if _, err = r.peekTag(); err != nil {
		t.Fatal(err)
	}

	if err = r.Skip(); err != nil || input.position != 2 {
		t.Fatalf("Skip after peek = %v, position %d", err, input.position)
	}

	if _, err = r.peekTag(); err != nil {
		t.Fatal(err)
	}

	r.Reset(bytes.NewReader([]byte{0xc3}))
	if value, err = r.DecodeBool(); err != nil || !value {
		t.Fatalf("DecodeBool after Reset = (%v, %v)", value, err)
	}
}

func requireStreamHeaderFailure(t *testing.T, d *Reader, want error) {
	t.Helper()
	array, arrayErr := d.DecodeArrayHeader()
	pairs, pairsErr := d.DecodeMapHeader()
	if array != 0 || pairs != 0 || arrayErr != want || pairsErr != want {
		t.Fatalf("stored header error = (%d, %v), (%d, %v), want %v", array, arrayErr, pairs, pairsErr, want)
	}

	requireReaderScalarFailure(t, d, want)
}
