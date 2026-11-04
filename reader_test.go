package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
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
