package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"runtime"
	"strings"
	"testing"
)

func TestFloat32Wire(t *testing.T) {
	type testcase struct {
		name string
		bits uint32
		wire []byte
	}
	tests := []testcase{
		{name: "positive zero", bits: 0x00000000, wire: []byte{0xca, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative zero", bits: 0x80000000, wire: []byte{0xca, 0x80, 0x00, 0x00, 0x00}},
		{name: "one and a half", bits: 0x3fc00000, wire: []byte{0xca, 0x3f, 0xc0, 0x00, 0x00}},
		{name: "negative two and a half", bits: 0xc0200000, wire: []byte{0xca, 0xc0, 0x20, 0x00, 0x00}},
		{name: "smallest subnormal", bits: 0x00000001, wire: []byte{0xca, 0x00, 0x00, 0x00, 0x01}},
		{name: "negative smallest subnormal", bits: 0x80000001, wire: []byte{0xca, 0x80, 0x00, 0x00, 0x01}},
		{name: "maximum finite", bits: 0x7f7fffff, wire: []byte{0xca, 0x7f, 0x7f, 0xff, 0xff}},
		{name: "minimum finite", bits: 0xff7fffff, wire: []byte{0xca, 0xff, 0x7f, 0xff, 0xff}},
		{name: "positive infinity", bits: 0x7f800000, wire: []byte{0xca, 0x7f, 0x80, 0x00, 0x00}},
		{name: "negative infinity", bits: 0xff800000, wire: []byte{0xca, 0xff, 0x80, 0x00, 0x00}},
		{name: "quiet NaN payload", bits: 0x7fc12345, wire: []byte{0xca, 0x7f, 0xc1, 0x23, 0x45}},
		{name: "negative quiet NaN payload", bits: 0xffc12345, wire: []byte{0xca, 0xff, 0xc1, 0x23, 0x45}},
		{name: "signaling NaN payload", bits: 0x7f812345, wire: []byte{0xca, 0x7f, 0x81, 0x23, 0x45}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				input     = math.Float32frombits(tt.bits)
				output    bytes.Buffer
				encodeErr = NewWriter(&output).EncodeFloat32(input)
			)
			if encodeErr != nil || !bytes.Equal(output.Bytes(), tt.wire) {
				t.Fatalf("EncodeFloat32(bits %x) = (%x, %v), want %x", tt.bits, output.Bytes(), encodeErr, tt.wire)
			}

			for _, trailing := range [][]byte{nil, {0xc3, 0xc1}} {
				var (
					src    = append(append([]byte(nil), tt.wire...), trailing...)
					before = append([]byte(nil), src...)
					value  float32
					wide   float64
					n      int
					err    error
				)
				sourceFloat32 := bytes.NewReader(src)
				value, err = NewReader(sourceFloat32).DecodeFloat32()
				n = len(src) - sourceFloat32.Len()
				if err != nil || n != len(tt.wire) || math.Float32bits(value) != tt.bits {
					t.Fatalf("DecodeFloat32(%x) = (bits %08x, %d, %v)", src, math.Float32bits(value), n, err)
				}

				if !bytes.Equal(src, before) {
					t.Fatal("DecodeFloat32() modified source bytes")
				}

				sourceFloat64 := bytes.NewReader(src)
				wide, err = NewReader(sourceFloat64).DecodeFloat64()
				n = len(src) - sourceFloat64.Len()
				if err != nil || n != len(tt.wire) {
					t.Fatalf("DecodeFloat64(%x) = (%v, %d, %v)", src, wide, n, err)
				}

				if math.IsNaN(float64(input)) {
					if !math.IsNaN(wide) {
						t.Fatalf("DecodeFloat64(%x) = %v, want NaN", src, wide)
					}
				} else if math.Float64bits(wide) != math.Float64bits(float64(input)) {
					t.Fatalf("DecodeFloat64(%x) bits = %016x, want %016x", src, math.Float64bits(wide), math.Float64bits(float64(input)))
				}

				if !bytes.Equal(src, before) {
					t.Fatal("DecodeFloat64() modified source bytes")
				}
			}
		})
	}
}

func TestFloat64Wire(t *testing.T) {
	type testcase struct {
		name string
		bits uint64
		wire []byte
	}
	tests := []testcase{
		{name: "positive zero", bits: 0x0000000000000000, wire: []byte{0xcb, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative zero", bits: 0x8000000000000000, wire: []byte{0xcb, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "one and a half", bits: 0x3ff8000000000000, wire: []byte{0xcb, 0x3f, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative two and a half", bits: 0xc004000000000000, wire: []byte{0xcb, 0xc0, 0x04, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "smallest subnormal", bits: 0x0000000000000001, wire: []byte{0xcb, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}},
		{name: "negative smallest subnormal", bits: 0x8000000000000001, wire: []byte{0xcb, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}},
		{name: "maximum finite", bits: 0x7fefffffffffffff, wire: []byte{0xcb, 0x7f, 0xef, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "minimum finite", bits: 0xffefffffffffffff, wire: []byte{0xcb, 0xff, 0xef, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "positive infinity", bits: 0x7ff0000000000000, wire: []byte{0xcb, 0x7f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative infinity", bits: 0xfff0000000000000, wire: []byte{0xcb, 0xff, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "quiet NaN payload", bits: 0x7ff8123456789abc, wire: []byte{0xcb, 0x7f, 0xf8, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
		{name: "negative quiet NaN payload", bits: 0xfff8123456789abc, wire: []byte{0xcb, 0xff, 0xf8, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
		{name: "signaling NaN payload", bits: 0x7ff0123456789abc, wire: []byte{0xcb, 0x7f, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				input     = math.Float64frombits(tt.bits)
				output    bytes.Buffer
				encodeErr = NewWriter(&output).EncodeFloat64(input)
			)
			if encodeErr != nil || !bytes.Equal(output.Bytes(), tt.wire) {
				t.Fatalf("EncodeFloat64(bits %x) = (%x, %v), want %x", tt.bits, output.Bytes(), encodeErr, tt.wire)
			}

			for _, trailing := range [][]byte{nil, {0xc3, 0xc1}} {
				var (
					src    = append(append([]byte(nil), tt.wire...), trailing...)
					before = append([]byte(nil), src...)
					value  float64
					narrow float32
					n      int
					err    error
				)
				sourceFloat64 := bytes.NewReader(src)
				value, err = NewReader(sourceFloat64).DecodeFloat64()
				n = len(src) - sourceFloat64.Len()
				if err != nil || n != len(tt.wire) || math.Float64bits(value) != tt.bits {
					t.Fatalf("DecodeFloat64(%x) = (bits %016x, %d, %v)", src, math.Float64bits(value), n, err)
				}

				if !bytes.Equal(src, before) {
					t.Fatal("DecodeFloat64() modified source bytes")
				}

				sourceFloat32 := bytes.NewReader(src)
				narrow, err = NewReader(sourceFloat32).DecodeFloat32()
				n = len(src) - sourceFloat32.Len()
				if !errors.Is(err, ErrTypeMismatch) || math.Float32bits(narrow) != 0 {
					t.Fatalf("DecodeFloat32(%x) = (%v, %d, %v), want (0, 0, ErrTypeMismatch)", src, narrow, n, err)
				}

				if !bytes.Equal(src, before) {
					t.Fatal("DecodeFloat32() modified source bytes")
				}
			}
		})
	}
}

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

func TestStreamContainerHeaders(t *testing.T) {
	tests := []struct {
		count uint32
		array []byte
		pairs []byte
	}{
		{count: 0, array: []byte{0x90}, pairs: []byte{0x80}},
		{count: 15, array: []byte{0x9f}, pairs: []byte{0x8f}},
		{count: 16, array: []byte{0xdc, 0x00, 0x10}, pairs: []byte{0xde, 0x00, 0x10}},
		{count: 65535, array: []byte{0xdc, 0xff, 0xff}, pairs: []byte{0xde, 0xff, 0xff}},
		{count: 65536, array: []byte{0xdd, 0x00, 0x01, 0x00, 0x00}, pairs: []byte{0xdf, 0x00, 0x01, 0x00, 0x00}},
		{count: math.MaxUint32, array: []byte{0xdd, 0xff, 0xff, 0xff, 0xff}, pairs: []byte{0xdf, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, tt := range tests {
		for _, isMap := range []bool{false, true} {
			t.Run(fmt.Sprintf("count_%d/map_%t", tt.count, isMap), func(t *testing.T) {
				var (
					writer bytes.Buffer
					e      = NewWriter(&writer)
					wire   = tt.array
					count  uint32
					err    error
				)
				if isMap {
					wire = tt.pairs
					err = e.EncodeMapHeader(tt.count)
				} else {
					err = e.EncodeArrayHeader(tt.count)
				}

				if err != nil || !bytes.Equal(writer.Bytes(), wire) {
					t.Fatalf("encoded header = (%x, %v), want %x", writer.Bytes(), err, wire)
				}

				r := &decoderTestReader{data: append(append([]byte(nil), wire...), 0xc3), maxRead: 1}
				d := NewReader(r)
				if err = d.SetLimits(ReaderLimits{MaxPayloadBytes: 1, MaxSkipDepth: 1, MaxSkipValues: 1, MaxSkipBytes: 1}); err != nil {
					t.Fatal(err)
				}

				if isMap {
					count, err = d.DecodeMapHeader()
				} else {
					count, err = d.DecodeArrayHeader()
				}

				if err != nil || count != tt.count || r.position != len(wire) {
					t.Fatalf("decoded header = (%d, %v), consumed %d", count, err, r.position)
				}

				if isMap && count == math.MaxUint32 && uint64(count)*2 != 8589934590 {
					t.Fatal("maximum map count did not preserve all child values")
				}

				value, nextErr := d.DecodeBool()
				if !value || nextErr != nil {
					t.Fatalf("following value = (%v, %v)", value, nextErr)
				}
			})
		}
	}
}

func TestStreamContainerHeaderIOFailures(t *testing.T) {
	failure := errors.New("container transport failure")
	for _, isMap := range []bool{false, true} {
		wire := []byte{0xdd, 0xff, 0xff, 0xff, 0xff}
		if isMap {
			wire[0] = 0xdf
		}

		for _, length := range []int{0, 1, 3, 5} {
			var (
				r     = &decoderTestReader{data: wire[:length], endErr: failure}
				d     = NewReader(r)
				count uint32
				err   error
			)
			if isMap {
				count, err = d.DecodeMapHeader()
			} else {
				count, err = d.DecodeArrayHeader()
			}

			if count != 0 || !errors.Is(err, failure) || err == failure || !strings.Contains(err.Error(), "source") {
				t.Fatalf("map %t source failure at %d = (%d, %v)", isMap, length, count, err)
			}

			reads := r.reads
			requireStreamHeaderFailure(t, d, err)
			if r.reads != reads {
				t.Fatal("source failure did not stop reads")
			}
		}
		for _, test := range []struct {
			limit int
			err   error
			want  error
		}{
			{limit: 0, err: failure, want: failure},
			{limit: 2, err: failure, want: failure},
			{limit: 5, err: failure, want: failure},
			{limit: 0, want: io.ErrShortWrite},
			{limit: 2, want: io.ErrShortWrite},
		} {
			var (
				w   = &encoderTestWriter{limited: true, limit: test.limit, err: test.err}
				e   = NewWriter(w)
				err error
			)
			if isMap {
				err = e.EncodeMapHeader(math.MaxUint32)
			} else {
				err = e.EncodeArrayHeader(math.MaxUint32)
			}

			if !errors.Is(err, test.want) || err == test.want || !strings.Contains(err.Error(), "destination") || !bytes.Equal(w.data, wire[:test.limit]) {
				t.Fatalf("map %t write limit %d = (%x, %v)", isMap, test.limit, w.data, err)
			}

			if e.EncodeArrayHeader(0) != err || e.EncodeMapHeader(0) != err || e.EncodeNil() != err || w.writes != 1 {
				t.Fatal("header write failure was not sticky")
			}
		}
	}
}

func TestStreamContainerHeaderInitializationAndReset(t *testing.T) {
	var (
		e Writer
		d Reader
		w bytes.Buffer
	)
	if e.EncodeArrayHeader(0) != ErrUninitialized || e.EncodeMapHeader(0) != ErrUninitialized {
		t.Fatal("unattached Writer headers did not return ErrUninitialized")
	}

	for _, decode := range []func() (count uint32, err error){d.DecodeArrayHeader, d.DecodeMapHeader} {
		count, err := decode()
		if count != 0 || err != ErrUninitialized {
			t.Fatalf("unattached Reader header = (%d, %v)", count, err)
		}
	}
	e.Reset(&w)
	if err := e.EncodeArrayHeader(0); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeMapHeader(0); err != nil || !bytes.Equal(w.Bytes(), []byte{0x90, 0x80}) {
		t.Fatalf("reset Writer headers = (%x, %v)", w.Bytes(), err)
	}

	d.Reset(bytes.NewReader(w.Bytes()))
	array, arrayErr := d.DecodeArrayHeader()
	pairs, pairsErr := d.DecodeMapHeader()
	if array != 0 || pairs != 0 || arrayErr != nil || pairsErr != nil {
		t.Fatalf("reset Reader headers = (%d, %v), (%d, %v)", array, arrayErr, pairs, pairsErr)
	}
}

func TestStreamTransferAllocationBytes(t *testing.T) {
	for _, operation := range []struct {
		name     string
		encode   bool
		isString bool
	}{
		{name: "EncodeBytes", encode: true},
		{name: "EncodeString", encode: true, isString: true},
		{name: "DecodeBytes"},
		{name: "DecodeString", isString: true},
	} {
		t.Run(operation.name, func(t *testing.T) {
			for _, reuse := range []bool{false, true} {
				var name = "New"
				if reuse {
					name = "Reset"
				}

				t.Run(name, func(t *testing.T) {
					var (
						minimum = ^uint64(0)
						maximum uint64
					)
					for _, size := range streamBenchmarkSizes {
						allocated := measureStreamTransferAllocation(t, size.length, operation.encode, operation.isString, reuse)
						t.Logf("%s: %d B/transfer over 16 transfers", size.name, allocated)
						// Four copy buffers allow allocator/runtime noise without
						// permitting even one payload-sized allocation at 8 MiB.
						if allocated > 4*payloadBufferSize {
							t.Fatalf("%s allocated %d B/transfer, budget %d", size.name, allocated, 4*payloadBufferSize)
						}

						if allocated < minimum {
							minimum = allocated
						}

						if allocated > maximum {
							maximum = allocated
						}
					}
					// This is a scaling bound, not an exact allocator-size promise.
					if maximum-minimum > 2*payloadBufferSize {
						t.Fatalf("allocation spread %d B/transfer exceeds %d", maximum-minimum, 2*payloadBufferSize)
					}
				})
			}
		})
	}
}

func measureStreamTransferAllocation(t *testing.T, length uint64, encode bool, isString bool, reuse bool) (bytesPerRun uint64) {
	t.Helper()
	var (
		source  streamBenchmarkPayloadReader
		sink    streamBenchmarkWriter
		encoder = NewWriter(&sink)
		decoder = NewReader(&source)
		before  runtime.MemStats
		after   runtime.MemStats
		run     func() (accepted uint64, transferErr error)
		n       uint64
		err     error
	)
	if encode {
		run = func() (accepted uint64, transferErr error) { return encoder.EncodeBytesFrom(length, &source) }
		if isString {
			run = func() (accepted uint64, transferErr error) { return encoder.EncodeStringFrom(length, &source) }
		}
	} else {
		run = func() (accepted uint64, transferErr error) { return decoder.DecodeBytesTo(&sink) }
		if isString {
			run = func() (accepted uint64, transferErr error) { return decoder.DecodeStringTo(&sink) }
		}
	}

	if reuse {
		source.Reset(length, isString, !encode)
		if n, err = run(); err != nil || n != length {
			t.Fatalf("warm transfer = %d, %v", n, err)
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&before)
	for index := 0; index < 16; index++ {
		source.Reset(length, isString, !encode)
		if encode {
			if reuse {
				encoder.Reset(&sink)
			} else {
				encoder = NewWriter(&sink)
				streamBenchmarkWriterSink = encoder
			}
		} else {
			if reuse {
				decoder.Reset(&source)
			} else {
				decoder = NewReader(&source)
				streamBenchmarkReaderSink = decoder
			}
		}

		if n, err = run(); err != nil || n != length {
			t.Fatalf("transfer = %d, %v", n, err)
		}
	}
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(encoder)
	runtime.KeepAlive(decoder)
	return (after.TotalAlloc - before.TotalAlloc) / 16
}

func TestStreamRetainedWorkingMemory(t *testing.T) {
	const cohort = 32
	for _, isString := range []bool{false, true} {
		var name = "Bytes"
		if isString {
			name = "String"
		}

		t.Run(name, func(t *testing.T) {
			for _, size := range streamBenchmarkSizes {
				t.Run(size.name, func(t *testing.T) {
					var (
						encoders = make([]*Writer, cohort)
						decoders = make([]*Reader, cohort)
						source   streamBenchmarkPayloadReader
						sink     streamBenchmarkWriter
						before   runtime.MemStats
						after    runtime.MemStats
						scratch  uint64
						n        uint64
						err      error
					)
					runtime.GC()
					runtime.ReadMemStats(&before)
					for index := 0; index < cohort; index++ {
						encoders[index] = NewWriter(&sink)
						decoders[index] = NewReader(&source)
						source.Reset(size.length, isString, false)
						if isString {
							n, err = encoders[index].EncodeStringFrom(size.length, &source)
						} else {
							n, err = encoders[index].EncodeBytesFrom(size.length, &source)
						}

						if err != nil || n != size.length {
							t.Fatalf("encode = %d, %v", n, err)
						}

						source.Reset(size.length, isString, true)
						if isString {
							n, err = decoders[index].DecodeStringTo(&sink)
						} else {
							n, err = decoders[index].DecodeBytesTo(&sink)
						}

						if err != nil || n != size.length {
							t.Fatalf("decode = %d, %v", n, err)
						}

						encoders[index].Reset(nil)
						decoders[index].Reset(nil)
						if cap(encoders[index].state.buffer) > payloadBufferSize || cap(decoders[index].state.buffer) > payloadBufferSize {
							t.Fatal("codec scratch capacity grew beyond the transfer bound")
						}

						scratch += uint64(cap(encoders[index].state.buffer) + cap(decoders[index].state.buffer))
					}
					runtime.GC()
					runtime.ReadMemStats(&after)
					// Keep every codec and its reachable storage live through the second GC.
					runtime.KeepAlive(encoders)
					runtime.KeepAlive(decoders)
					delta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
					t.Logf("%d live Writer/Reader pairs: scratch=%d B, post-GC heap delta=%d B (%d B/pair)", cohort, scratch, delta, delta/cohort)
					// Heap deltas include process/runtime noise. Allow two times the
					// bounded scratch plus 1 MiB; do not assert an exact live heap size.
					budget := int64(cohort*4*payloadBufferSize + 1048576)
					if delta > budget {
						t.Fatalf("retained heap delta %d exceeds broad %d-byte budget", delta, budget)
					}
				})
			}
		})
	}
}

func TestStreamTransferScratchBounds(t *testing.T) {
	var (
		source  streamBenchmarkPayloadReader
		sink    streamBenchmarkWriter
		encoder = NewWriter(&sink)
		decoder = NewReader(&source)
		n       uint64
		err     error
	)
	for _, size := range streamBenchmarkSizes {
		for _, isString := range []bool{false, true} {
			source.Reset(size.length, isString, false)
			encoder.Reset(&sink)
			if isString {
				n, err = encoder.EncodeStringFrom(size.length, &source)
			} else {
				n, err = encoder.EncodeBytesFrom(size.length, &source)
			}

			if err != nil || n != size.length {
				t.Fatalf("encode = %d, %v", n, err)
			}

			source.Reset(size.length, isString, true)
			decoder.Reset(&source)
			if isString {
				n, err = decoder.DecodeStringTo(&sink)
			} else {
				n, err = decoder.DecodeBytesTo(&sink)
			}

			if err != nil || n != size.length {
				t.Fatalf("decode = %d, %v", n, err)
			}

			if cap(encoder.state.buffer) > payloadBufferSize || cap(decoder.state.buffer) > payloadBufferSize {
				t.Fatal("unexpected retained scratch bounds")
			}
		}
	}
}

func TestStreamNestedContainers(t *testing.T) {
	var (
		writer bytes.Buffer
		e      = NewWriter(&writer)
		steps  = []func() (err error){
			func() (err error) { return e.EncodeArrayHeader(2) },
			func() (err error) { return e.EncodeMapHeader(1) },
			func() (err error) { return e.EncodeString("k") },
			func() (err error) { return e.EncodeArrayHeader(2) },
			e.EncodeNil,
			func() (err error) { return e.EncodeMapHeader(0) },
			func() (err error) { return e.EncodeArrayHeader(0) },
			func() (err error) { return e.EncodeBool(true) },
		}
		wire = []byte{0x92, 0x81, 0xa1, 'k', 0x92, 0xc0, 0x80, 0x90, 0xc3}
	)
	for _, step := range steps {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(writer.Bytes(), wire) {
		t.Fatalf("nested container bytes = %x, want %x", writer.Bytes(), wire)
	}

	d := NewReader(bytes.NewReader(wire))
	if count, err := d.DecodeArrayHeader(); count != 2 || err != nil {
		t.Fatalf("outer array = (%d, %v)", count, err)
	}

	if count, err := d.DecodeMapHeader(); count != 1 || err != nil {
		t.Fatalf("nested map = (%d, %v)", count, err)
	}

	if key, err := d.DecodeString(); key != "k" || err != nil {
		t.Fatalf("nested key = (%q, %v)", key, err)
	}

	if count, err := d.DecodeArrayHeader(); count != 2 || err != nil {
		t.Fatalf("nested array = (%d, %v)", count, err)
	}

	if err := d.DecodeNil(); err != nil {
		t.Fatal(err)
	}

	if count, err := d.DecodeMapHeader(); count != 0 || err != nil {
		t.Fatalf("empty map = (%d, %v)", count, err)
	}

	if count, err := d.DecodeArrayHeader(); count != 0 || err != nil {
		t.Fatalf("empty array = (%d, %v)", count, err)
	}

	if value, err := d.DecodeBool(); !value || err != nil {
		t.Fatalf("following value = (%v, %v)", value, err)
	}
}

func TestStreamGeneratedArrayIsIncremental(t *testing.T) {
	const count uint32 = 100000
	var (
		writer = &streamArrayWriter{count: count}
		e      = NewWriter(writer)
		r      = newStreamArrayReader(count)
		d      = NewReader(r)
		total  uint64
	)
	if err := e.EncodeArrayHeader(count); err != nil {
		t.Fatal(err)
	}

	for value := uint64(0); value < uint64(count); value++ {
		if err := e.EncodeUint64(value); err != nil {
			t.Fatal(err)
		}

		if value == 0 && (writer.seen != 1 || writer.writes != 2) {
			t.Fatal("Writer delayed the first element until the array was complete")
		}
	}
	if writer.seen != uint64(count) || writer.maxRequest > 9 || e.state.buffer != nil {
		t.Fatalf("generated writes = %d values, max request %d, buffer %d", writer.seen, writer.maxRequest, len(e.state.buffer))
	}

	decodedCount, headerErr := d.DecodeArrayHeader()
	if headerErr != nil || decodedCount != count || r.next != 0 {
		t.Fatalf("generated array header = (%d, %v), generated elements %d", decodedCount, headerErr, r.next)
	}

	for index := uint64(0); index < uint64(decodedCount); index++ {
		value, readErr := d.DecodeUint64()
		if readErr != nil || value != index {
			t.Fatalf("generated element %d = (%d, %v)", index, value, readErr)
		}

		total += value
		if index == 0 && r.next != 1 {
			t.Fatal("Reader waited for ungenerated elements before returning the first")
		}
	}
	if total != uint64(count)*(uint64(count)-1)/2 || r.bytes != writer.bytes || r.maxRequest > 8 || d.state.buffer != nil {
		t.Fatalf("incremental total %d, wire bytes %d/%d, request %d, buffer %d", total, r.bytes, writer.bytes, r.maxRequest, len(d.state.buffer))
	}

	if err := d.DecodeNil(); !errors.Is(err, io.EOF) {
		t.Fatalf("generated array end = %v", err)
	}
}
