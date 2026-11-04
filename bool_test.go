package messagepack

import (
	"bytes"
	"errors"
	"testing"
)

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
