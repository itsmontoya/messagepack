package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestMapHeaderWire(t *testing.T) {
	type testcase struct {
		name  string
		count uint32
		wire  []byte
	}
	tests := []testcase{
		{name: "empty", wire: []byte{0x80}},
		{name: "one", count: 1, wire: []byte{0x81}},
		{name: "maximum fixmap", count: 15, wire: []byte{0x8f}},
		{name: "above fixmap", count: 16, wire: []byte{0xde, 0x00, 0x10}},
		{name: "maximum map16", count: 65535, wire: []byte{0xde, 0xff, 0xff}},
		{name: "above map16", count: 65536, wire: []byte{0xdf, 0x00, 0x01, 0x00, 0x00}},
		{name: "maximum uint32", count: 4294967295, wire: []byte{0xdf, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				output    bytes.Buffer
				encodeErr = NewWriter(&output).EncodeMapHeader(tt.count)
			)
			if encodeErr != nil || !bytes.Equal(output.Bytes(), tt.wire) {
				t.Fatalf("EncodeMapHeader(%d) = (%x, %v), want %x", tt.count, output.Bytes(), encodeErr, tt.wire)
			}

			for _, trailing := range [][]byte{nil, {0xc3, 0xc1}} {
				var (
					src    = append(append([]byte(nil), tt.wire...), trailing...)
					before = append([]byte(nil), src...)
					count  uint32
					n      int
					err    error
				)
				sourceMapHeader := bytes.NewReader(src)
				count, err = NewReader(sourceMapHeader).DecodeMapHeader()
				n = len(src) - sourceMapHeader.Len()
				if err != nil || count != tt.count || n != len(tt.wire) {
					t.Fatalf("DecodeMapHeader(%x) = (%d, %d, %v), want (%d, %d, nil)", src, count, n, err, tt.count, len(tt.wire))
				}

				if !bytes.Equal(src, before) {
					t.Fatal("DecodeMapHeader modified input")
				}
			}
		})
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
