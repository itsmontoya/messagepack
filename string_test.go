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
				wire      = append(append([]byte(nil), tt.header...), tt.value...)
				reused    = make([]byte, 2, len(wire)+2)
				scenarios = []struct {
					name  string
					dst   []byte
					reuse bool
				}{
					{name: "fresh"},
					{name: "prefix growth", dst: []byte{0xaa, 0xbb}},
					{name: "prefix reuse", dst: reused, reuse: true},
					{name: "reset", dst: make([]byte, 0, len(wire)), reuse: true},
				}
			)
			reused[0], reused[1] = 0xaa, 0xbb
			for _, scenario := range scenarios {
				t.Run(scenario.name, func(t *testing.T) {
					var (
						want = append(append([]byte(nil), scenario.dst...), wire...)
						out  []byte
						err  error
					)
					out, err = AppendString(scenario.dst, tt.value)
					if err != nil || !bytes.Equal(out, want) {
						t.Fatalf("AppendString() = (%x, %v), want %x", out, err, want)
					}
					if scenario.reuse && &out[0] != &scenario.dst[:cap(scenario.dst)][0] {
						t.Fatal("AppendString did not reuse destination capacity")
					}
				})
			}

			var (
				src    = append(append([]byte(nil), wire...), 0xc3)
				before = append([]byte(nil), src...)
				owned  string
				view   []byte
				n      int
				err    error
			)
			owned, n, err = ReadString(src)
			if err != nil || owned != tt.value || n != len(wire) {
				t.Fatalf("ReadString() = (%q, %d, %v), want (%q, %d, nil)", owned, n, err, tt.value, len(wire))
			}
			view, n, err = ReadStringBytes(src)
			if err != nil || string(view) != tt.value || n != len(wire) || cap(view) != len(view) {
				t.Fatalf("ReadStringBytes() = (%x, %d, %v), cap %d", view, n, err, cap(view))
			}
			if !bytes.Equal(src, before) || src[n] != 0xc3 {
				t.Fatal("string reader modified input or consumed trailing value")
			}
		})
	}
}

func TestReadStringWiderHeaders(t *testing.T) {
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
				view  []byte
				n     int
				err   error
			)
			owned, n, err = ReadString(tt.src)
			if err != nil || owned != tt.want || n != len(tt.src) {
				t.Fatalf("ReadString(%x) = (%q, %d, %v)", tt.src, owned, n, err)
			}
			view, n, err = ReadStringBytes(tt.src)
			if err != nil || string(view) != tt.want || n != len(tt.src) || cap(view) != len(view) {
				t.Fatalf("ReadStringBytes(%x) = (%x, %d, %v), cap %d", tt.src, view, n, err, cap(view))
			}
		})
	}
}

func TestReadStringErrors(t *testing.T) {
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
		{name: "huge below max int32", src: []byte{0xdb, 0x7f, 0xff, 0xff, 0xff, 0x78}, err: ErrTruncated},
		{name: "huge above max int32", src: []byte{0xdb, 0x80, 0x00, 0x00, 0x00, 0x78}, err: ErrTruncated},
		{name: "maximum wire length", src: []byte{0xdb, 0xff, 0xff, 0xff, 0xff, 0x78}, err: ErrTruncated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before = append([]byte(nil), tt.src...)
				owned  string
				view   []byte
				n      int
				err    error
			)
			owned, n, err = ReadString(tt.src)
			if owned != "" || n != 0 || !errors.Is(err, tt.err) {
				t.Fatalf("ReadString(%x) = (%q, %d, %v), want zero and %v", tt.src, owned, n, err, tt.err)
			}
			view, n, err = ReadStringBytes(tt.src)
			if view != nil || n != 0 || !errors.Is(err, tt.err) {
				t.Fatalf("ReadStringBytes(%x) = (%x, %d, %v), want nil, 0, %v", tt.src, view, n, err, tt.err)
			}
			if !bytes.Equal(tt.src, before) {
				t.Fatal("string reader modified input")
			}
		})
	}
}

func TestReadStringsTruncated(t *testing.T) {
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
					view   []byte
					n      int
					err    error
				)
				owned, n, err = ReadString(src)
				if owned != "" || n != 0 || !errors.Is(err, ErrTruncated) {
					t.Fatalf("ReadString(%x) = (%q, %d, %v)", src, owned, n, err)
				}
				view, n, err = ReadStringBytes(src)
				if view != nil || n != 0 || !errors.Is(err, ErrTruncated) {
					t.Fatalf("ReadStringBytes(%x) = (%x, %d, %v)", src, view, n, err)
				}
				if !bytes.Equal(wire, before) {
					t.Fatal("string reader modified source backing storage")
				}
			})
		}
	}
}

func TestStringOwnership(t *testing.T) {
	var (
		src   = []byte{0xa3, 0x61, 0x62, 0x63, 0xc3}
		owned string
		view  []byte
		n     int
		err   error
	)
	owned, n, err = ReadString(src)
	if err != nil || n != 4 || owned != "abc" {
		t.Fatalf("ReadString() = (%q, %d, %v)", owned, n, err)
	}
	view, n, err = ReadStringBytes(src)
	if err != nil || n != 4 || cap(view) != 3 || &view[0] != &src[1] {
		t.Fatalf("ReadStringBytes() did not return a capped borrowed view: (%x, %d, %v)", view, n, err)
	}
	src[1] = 0x7a
	if string(view) != "zbc" || owned != "abc" {
		t.Fatalf("source mutation: view %q, owned %q", view, owned)
	}
	view[1] = 0x79
	if src[2] != 0x79 || owned != "abc" {
		t.Fatal("view mutation did not alias source or altered owned string")
	}
	view = append(view, 0x21)
	if string(view) != "zyc!" || src[4] != 0xc3 {
		t.Fatal("appending to view overwrote trailing encoded value")
	}
}

func TestAppendStringHeaderLimits(t *testing.T) {
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
				storage = []byte{0xaa, 0xbb, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66}
				before  = append([]byte(nil), storage...)
				dst     = storage[:2]
				out     []byte
				err     error
			)
			out, err = appendStringHeader(dst, tt.length)
			if !bytes.Equal(out, tt.want) || !errors.Is(err, tt.err) {
				t.Fatalf("appendStringHeader() = (%x, %v), want (%x, %v)", out, err, tt.want, tt.err)
			}
			if tt.err != nil && (!bytes.Equal(storage, before) || &out[0] != &dst[0] || cap(out) != cap(dst)) {
				t.Fatal("overflow changed destination slice or backing storage")
			}
		})
	}
}
