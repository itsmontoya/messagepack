package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"testing"
)

func TestDecoderPayloadWire(t *testing.T) {
	tests := []struct {
		name         string
		length       int
		stringHeader []byte
		binaryHeader []byte
	}{
		{name: "empty", stringHeader: []byte{0xa0}, binaryHeader: []byte{0xc4, 0}},
		{name: "one", length: 1, stringHeader: []byte{0xa1}, binaryHeader: []byte{0xc4, 1}},
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
			owned  func(d *Decoder) (out []byte, err error)
			copy   func(d *Decoder, dst io.Writer) (n uint64, err error)
		}{
			{name: "string", header: tt.stringHeader, owned: func(d *Decoder) (out []byte, err error) {
				var text string
				text, err = d.DecodeString()
				return []byte(text), err
			}, copy: (*Decoder).DecodeStringTo},
			{name: "binary", header: tt.binaryHeader, owned: (*Decoder).DecodeBytes, copy: (*Decoder).DecodeBytesTo},
		}
		for _, family := range families {
			t.Run(tt.name+"/"+family.name, func(t *testing.T) {
				var (
					wire = append(append(append([]byte(nil), family.header...), value...), 0xc3)
					r    = &decoderTestReader{data: wire, maxRead: 7}
					d    = NewDecoder(r)
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

func TestDecoderPayloadWiderHeadersAndRawBytes(t *testing.T) {
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
				d     = NewDecoder(r)
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

func TestDecoderPayloadTruncation(t *testing.T) {
	tests := []struct {
		wire     []byte
		header   int
		isString bool
	}{
		{wire: []byte{0xa2, 1, 2}, header: 1, isString: true},
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
					d     = NewDecoder(r)
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
				if owned != nil || !errors.Is(err, want) || errors.Is(err, ErrTruncated) || r.position != length {
					t.Fatalf("owned truncation = (%x, %v), consumed %d", owned, err, r.position)
				}
				requireDecoderPayloadFailure(t, d, err)
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
				if n != uint64(wantN) || dst.Len() != wantN || !errors.Is(err, want) || errors.Is(err, ErrTruncated) || r.position != length {
					t.Fatalf("transfer truncation = (%d, %v), bytes %d, consumed %d", n, err, dst.Len(), r.position)
				}
				reads := r.reads
				requireDecoderPayloadFailure(t, d, err)
				if r.reads != reads {
					t.Fatal("sticky payload failure performed more reads")
				}
			})
		}
	}
}

func requireDecoderPayloadFailure(t *testing.T, d *Decoder, want error) {
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
	requireDecoderScalarFailure(t, d, want)
}

func TestDecoderPayloadLimits(t *testing.T) {
	for _, header := range [][]byte{{0xdb, 0xff, 0xff, 0xff, 0xff}, {0xc6, 0xff, 0xff, 0xff, 0xff}} {
		r := &decoderTestReader{data: append(append([]byte(nil), header...), 0x78)}
		d := NewDecoder(r)
		var err error
		if header[0] == 0xdb {
			_, err = d.DecodeString()
		} else {
			_, err = d.DecodeBytes()
		}
		if err != ErrLimitExceeded || r.position != len(header) || d.buffer != nil {
			t.Fatalf("hostile length = %v, consumed %d, buffer %d", err, r.position, len(d.buffer))
		}
		requireDecoderPayloadFailure(t, d, err)
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
			d := NewDecoder(r)
			if err := d.SetLimits(DecoderLimits{MaxPayloadBytes: 2}); err != nil {
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

func TestDecoderOwnedPayloadIntOverflow(t *testing.T) {
	if strconv.IntSize != 32 {
		t.Skip("uint32 payload lengths fit int on this platform; 32-bit execution required")
	}
	for _, tag := range []byte{0xdb, 0xc6} {
		r := &decoderTestReader{data: []byte{tag, 0x80, 0, 0, 0, 0x78}}
		d := NewDecoder(r)
		if err := d.SetLimits(DecoderLimits{MaxPayloadBytes: math.MaxUint32}); err != nil {
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

func TestDecoderPayloadOwnership(t *testing.T) {
	var (
		wire = []byte{0xa3, 'a', 'b', 'c', 0xc4, 3, 1, 2, 3, 0xa1, 'z', 0xc4, 1, 9}
		d    = NewDecoder(bytes.NewReader(wire))
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
	later, err := d.DecodeBytes()
	if err != nil {
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
