package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestArrayHeaderWire(t *testing.T) {
	type testcase struct {
		name  string
		count uint32
		wire  []byte
	}
	tests := []testcase{
		{name: "empty", wire: []byte{0x90}},
		{name: "one", count: 1, wire: []byte{0x91}},
		{name: "maximum fixarray", count: 15, wire: []byte{0x9f}},
		{name: "above fixarray", count: 16, wire: []byte{0xdc, 0x00, 0x10}},
		{name: "maximum array16", count: 65535, wire: []byte{0xdc, 0xff, 0xff}},
		{name: "above array16", count: 65536, wire: []byte{0xdd, 0x00, 0x01, 0x00, 0x00}},
		{name: "maximum uint32", count: 4294967295, wire: []byte{0xdd, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				output    bytes.Buffer
				encodeErr = NewWriter(&output).EncodeArrayHeader(tt.count)
			)
			if encodeErr != nil || !bytes.Equal(output.Bytes(), tt.wire) {
				t.Fatalf("EncodeArrayHeader(%d) = (%x, %v), want %x", tt.count, output.Bytes(), encodeErr, tt.wire)
			}
			for _, trailing := range [][]byte{nil, {0xc3, 0xc1}} {
				var (
					src    = append(append([]byte(nil), tt.wire...), trailing...)
					before = append([]byte(nil), src...)
					count  uint32
					n      int
					err    error
				)
				sourceArrayHeader := bytes.NewReader(src)
				count, err = NewReader(sourceArrayHeader).DecodeArrayHeader()
				n = len(src) - sourceArrayHeader.Len()
				if err != nil || count != tt.count || n != len(tt.wire) {
					t.Fatalf("DecodeArrayHeader(%x) = (%d, %d, %v), want (%d, %d, nil)", src, count, n, err, tt.count, len(tt.wire))
				}
				if !bytes.Equal(src, before) {
					t.Fatal("DecodeArrayHeader modified input")
				}
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
