package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestStringWireFixtures(t *testing.T) {
	type testcase struct {
		name   string
		value  string
		header []byte
	}
	tests := []testcase{
		{name: "empty", header: []byte{0xa0}},
		{name: "one", value: "x", header: []byte{0xa1}},
		{name: "31 bytes", value: strings.Repeat("x", 31), header: []byte{0xbf}},
		{name: "32 bytes", value: strings.Repeat("x", 32), header: []byte{0xd9, 0x20}},
		{name: "255 bytes", value: strings.Repeat("x", 255), header: []byte{0xd9, 0xff}},
		{name: "256 bytes", value: strings.Repeat("x", 256), header: []byte{0xda, 0x01, 0x00}},
		{name: "65535 bytes", value: strings.Repeat("x", 65535), header: []byte{0xda, 0xff, 0xff}},
		{name: "65536 bytes", value: strings.Repeat("x", 65536), header: []byte{0xdb, 0x00, 0x01, 0x00, 0x00}},
		{name: "NUL multibyte and invalid UTF-8", value: "\x00é\xff", header: []byte{0xa4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				wire   = append(append([]byte(nil), tt.header...), tt.value...)
				output bytes.Buffer
				err    = NewWriter(&output).EncodeString(tt.value)
			)
			if err != nil || !bytes.Equal(output.Bytes(), wire) {
				t.Fatalf("EncodeString() = (%x, %v), want %x", output.Bytes(), err, wire)
			}

			var (
				src    = append(append([]byte(nil), wire...), 0xc3)
				before = append([]byte(nil), src...)
				owned  string
				n      int
			)
			sourceString := bytes.NewReader(src)
			owned, err = NewReader(sourceString).DecodeString()
			n = len(src) - sourceString.Len()
			if err != nil || owned != tt.value || n != len(wire) {
				t.Fatalf("DecodeString() = (%q, %d, %v), want (%q, %d, nil)", owned, n, err, tt.value, len(wire))
			}

			if !bytes.Equal(src, before) || src[n] != 0xc3 {
				t.Fatal("string reader modified input or consumed trailing value")
			}
		})
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

func TestWriterStringHeaderLimits(t *testing.T) {
	type testcase struct {
		name   string
		length uint64
		want   []byte
		err    error
	}
	tests := []testcase{
		{name: "maximum wire length", length: 4294967295, want: []byte{0xaa, 0xbb, 0xdb, 0xff, 0xff, 0xff, 0xff}},
		{name: "above wire limit", length: 4294967296, want: []byte{0xaa, 0xbb}, err: ErrOverflow},
		{name: "maximum uint64", length: ^uint64(0), want: []byte{0xaa, 0xbb}, err: ErrOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				output = bytes.NewBuffer([]byte{0xaa, 0xbb})
				w      = NewWriter(output)
				err    = w.writePayloadHeader("encode string header", tt.length, true)
			)
			if !bytes.Equal(output.Bytes(), tt.want) || !errors.Is(err, tt.err) {
				t.Fatalf("string header = (%x, %v), want (%x, %v)", output.Bytes(), err, tt.want, tt.err)
			}

			if tt.err != nil {
				if err = w.EncodeBool(true); err != nil || !bytes.Equal(output.Bytes(), []byte{0xaa, 0xbb, 0xc3}) {
					t.Fatalf("EncodeBool after overflow = (%x, %v)", output.Bytes(), err)
				}
			}
		})
	}
}
