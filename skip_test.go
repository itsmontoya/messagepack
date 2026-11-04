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
		{name: "str8", wire: []byte{0xd9, 2, 0xff, 0}},
		{name: "str16", wire: []byte{0xda, 0, 2, 0xff, 0}},
		{name: "str32", wire: []byte{0xdb, 0, 0, 0, 2, 0xff, 0}},
		{name: "empty str8", wire: []byte{0xd9, 0}},
		{name: "empty str16", wire: []byte{0xda, 0, 0}},
		{name: "empty str32", wire: []byte{0xdb, 0, 0, 0, 0}},
		{name: "bin8", wire: []byte{0xc4, 2, 0xff, 0}},
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
		{0xc4, 2, 0xff, 0}, {0xc5, 0, 2, 0xff, 0}, {0xc6, 0, 0, 0, 2, 0xff, 0},
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
