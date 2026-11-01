package messagepack

import (
	"bytes"
	"errors"
	"testing"
)

func TestAppendNil(t *testing.T) {
	type testcase struct {
		name  string
		dst   []byte
		want  []byte
		reuse bool
	}
	tests := []testcase{
		{name: "fresh", want: []byte{0xc0}},
		{name: "prefix with growth", dst: []byte{0xaa, 0xbb}, want: []byte{0xaa, 0xbb, 0xc0}},
		{name: "prefix with capacity", dst: []byte{0xaa, 0xbb, 0, 0}[:2], want: []byte{0xaa, 0xbb, 0xc0}, reuse: true},
		{name: "reset buffer", dst: []byte{0xff, 0xff}[:0], want: []byte{0xc0}, reuse: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := AppendNil(tt.dst)
			if !bytes.Equal(out, tt.want) {
				t.Fatalf("AppendNil() = %x, want %x", out, tt.want)
			}
			if tt.reuse && &out[0] != &tt.dst[:cap(tt.dst)][0] {
				t.Fatal("AppendNil() did not reuse available destination capacity")
			}
		})
	}
}

func TestReadNil(t *testing.T) {
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
			n, err = ReadNil(tt.src)
			if n != tt.n || !errors.Is(err, tt.err) {
				t.Fatalf("ReadNil(%x) = (%d, %v), want (%d, %v)", tt.src, n, err, tt.n, tt.err)
			}
			if !bytes.Equal(tt.src, before) {
				t.Fatal("ReadNil() modified source bytes")
			}
		})
	}
}
