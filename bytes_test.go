package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestBytesWireFixtures(t *testing.T) {
	type testcase struct {
		name   string
		value  []byte
		header []byte
	}
	tests := []testcase{
		{name: "nil is empty binary", header: []byte{0xc4, 0x00}},
		{name: "empty", value: []byte{}, header: []byte{0xc4, 0x00}},
		{name: "one", value: []byte{0x78}, header: []byte{0xc4, 0x01}},
		{name: "31 bytes", value: bytes.Repeat([]byte{0x78}, 31), header: []byte{0xc4, 0x1f}},
		{name: "32 bytes", value: bytes.Repeat([]byte{0x78}, 32), header: []byte{0xc4, 0x20}},
		{name: "255 bytes", value: bytes.Repeat([]byte{0x78}, 255), header: []byte{0xc4, 0xff}},
		{name: "256 bytes", value: bytes.Repeat([]byte{0x78}, 256), header: []byte{0xc5, 0x01, 0x00}},
		{name: "65535 bytes", value: bytes.Repeat([]byte{0x78}, 65535), header: []byte{0xc5, 0xff, 0xff}},
		{name: "65536 bytes", value: bytes.Repeat([]byte{0x78}, 65536), header: []byte{0xc6, 0x00, 0x01, 0x00, 0x00}},
		{name: "raw bytes", value: []byte{0x00, 0xc3, 0xa9, 0xff}, header: []byte{0xc4, 0x04}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				wire   = append(append([]byte(nil), tt.header...), tt.value...)
				output bytes.Buffer
				err    = NewWriter(&output).EncodeBytes(tt.value)
			)
			if err != nil || !bytes.Equal(output.Bytes(), wire) {
				t.Fatalf("EncodeBytes() = (%x, %v), want %x", output.Bytes(), err, wire)
			}

			var (
				src    = append(append([]byte(nil), wire...), 0xc3)
				before = append([]byte(nil), src...)
				view   []byte
				n      int
			)
			sourceBytes := bytes.NewReader(src)
			view, err = NewReader(sourceBytes).DecodeBytes()
			n = len(src) - sourceBytes.Len()
			if err != nil || !bytes.Equal(view, tt.value) || n != len(wire) || cap(view) != len(view) {
				t.Fatalf("DecodeBytes() = (%x, %d, %v), cap %d", view, n, err, cap(view))
			}

			if !bytes.Equal(src, before) || src[n] != 0xc3 {
				t.Fatal("DecodeBytes modified input or consumed trailing value")
			}
		})
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

func TestWriterBytesHeaderLimits(t *testing.T) {
	type testcase struct {
		name   string
		length uint64
		want   []byte
		err    error
	}
	tests := []testcase{
		{name: "maximum wire length", length: 4294967295, want: []byte{0xaa, 0xbb, 0xc6, 0xff, 0xff, 0xff, 0xff}},
		{name: "above wire limit", length: 4294967296, want: []byte{0xaa, 0xbb}, err: ErrOverflow},
		{name: "maximum uint64", length: ^uint64(0), want: []byte{0xaa, 0xbb}, err: ErrOverflow},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				output = bytes.NewBuffer([]byte{0xaa, 0xbb})
				w      = NewWriter(output)
				err    = w.writePayloadHeader("encode bytes header", tt.length, false)
			)
			if !bytes.Equal(output.Bytes(), tt.want) || !errors.Is(err, tt.err) {
				t.Fatalf("bytes header = (%x, %v), want (%x, %v)", output.Bytes(), err, tt.want, tt.err)
			}

			if tt.err != nil {
				if err = w.EncodeBool(true); err != nil || !bytes.Equal(output.Bytes(), []byte{0xaa, 0xbb, 0xc3}) {
					t.Fatalf("EncodeBool after overflow = (%x, %v)", output.Bytes(), err)
				}
			}
		})
	}
}
