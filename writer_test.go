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

func TestWriterNilAndBoolWire(t *testing.T) {
	tests := []struct {
		name string
		run  func(e *Writer) (err error)
		wire []byte
	}{
		{name: "nil", run: (*Writer).EncodeNil, wire: []byte{0xc0}},
		{name: "false", run: func(e *Writer) (err error) { return e.EncodeBool(false) }, wire: []byte{0xc2}},
		{name: "true", run: func(e *Writer) (err error) { return e.EncodeBool(true) }, wire: []byte{0xc3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				writer bytes.Buffer
				e      = NewWriter(&writer)
				err    = tt.run(e)
			)

			if err != nil || !bytes.Equal(writer.Bytes(), tt.wire) {
				t.Fatalf("encoding = (%x, %v), want (%x, nil)", writer.Bytes(), err, tt.wire)
			}
		})
	}
}

func TestWriterNonnegativeIntegerWire(t *testing.T) {
	tests := []struct {
		name  string
		value uint64
		wire  []byte
	}{
		{name: "zero", value: 0, wire: []byte{0x00}},
		{name: "positive fixint maximum", value: 127, wire: []byte{0x7f}},
		{name: "uint8 minimum", value: 128, wire: []byte{0xcc, 0x80}},
		{name: "uint8 maximum", value: 255, wire: []byte{0xcc, 0xff}},
		{name: "uint16 minimum", value: 256, wire: []byte{0xcd, 0x01, 0x00}},
		{name: "uint16 maximum", value: 65535, wire: []byte{0xcd, 0xff, 0xff}},
		{name: "uint32 minimum", value: 65536, wire: []byte{0xce, 0x00, 0x01, 0x00, 0x00}},
		{name: "uint32 maximum", value: 4294967295, wire: []byte{0xce, 0xff, 0xff, 0xff, 0xff}},
		{name: "uint64 minimum", value: 4294967296, wire: []byte{0xcf, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}},
		{name: "int64 maximum", value: math.MaxInt64, wire: []byte{0xcf, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "uint64 maximum", value: math.MaxUint64, wire: []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				writer bytes.Buffer
				e      = NewWriter(&writer)
				err    = e.EncodeUint64(tt.value)
			)

			if err != nil || !bytes.Equal(writer.Bytes(), tt.wire) {
				t.Fatalf("EncodeUint64(%d) = (%x, %v), want (%x, nil)", tt.value, writer.Bytes(), err, tt.wire)
			}

			if tt.value > math.MaxInt64 {
				return
			}

			writer.Reset()
			err = e.EncodeInt64(int64(tt.value))
			if err != nil || !bytes.Equal(writer.Bytes(), tt.wire) {
				t.Fatalf("EncodeInt64(%d) = (%x, %v), want (%x, nil)", tt.value, writer.Bytes(), err, tt.wire)
			}
		})
	}
}

func TestWriterNegativeIntegerWire(t *testing.T) {
	tests := []struct {
		name  string
		value int64
		wire  []byte
	}{
		{name: "negative fixint maximum", value: -1, wire: []byte{0xff}},
		{name: "negative fixint minimum", value: -32, wire: []byte{0xe0}},
		{name: "int8 maximum", value: -33, wire: []byte{0xd0, 0xdf}},
		{name: "int8 minimum", value: -128, wire: []byte{0xd0, 0x80}},
		{name: "int16 maximum", value: -129, wire: []byte{0xd1, 0xff, 0x7f}},
		{name: "int16 minimum", value: -32768, wire: []byte{0xd1, 0x80, 0x00}},
		{name: "int32 maximum", value: -32769, wire: []byte{0xd2, 0xff, 0xff, 0x7f, 0xff}},
		{name: "int32 minimum", value: -2147483648, wire: []byte{0xd2, 0x80, 0x00, 0x00, 0x00}},
		{name: "int64 maximum negative encoding", value: -2147483649, wire: []byte{0xd3, 0xff, 0xff, 0xff, 0xff, 0x7f, 0xff, 0xff, 0xff}},
		{name: "int64 minimum", value: math.MinInt64, wire: []byte{0xd3, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				writer bytes.Buffer
				e      = NewWriter(&writer)
				err    = e.EncodeInt64(tt.value)
			)

			if err != nil || !bytes.Equal(writer.Bytes(), tt.wire) {
				t.Fatalf("EncodeInt64(%d) = (%x, %v), want (%x, nil)", tt.value, writer.Bytes(), err, tt.wire)
			}
		})
	}
}

func TestWriterFloat32Wire(t *testing.T) {
	tests := []struct {
		name string
		bits uint32
		wire []byte
	}{
		{name: "positive zero", bits: 0x00000000, wire: []byte{0xca, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative zero", bits: 0x80000000, wire: []byte{0xca, 0x80, 0x00, 0x00, 0x00}},
		{name: "one and a half", bits: 0x3fc00000, wire: []byte{0xca, 0x3f, 0xc0, 0x00, 0x00}},
		{name: "smallest subnormal", bits: 0x00000001, wire: []byte{0xca, 0x00, 0x00, 0x00, 0x01}},
		{name: "maximum finite", bits: 0x7f7fffff, wire: []byte{0xca, 0x7f, 0x7f, 0xff, 0xff}},
		{name: "positive infinity", bits: 0x7f800000, wire: []byte{0xca, 0x7f, 0x80, 0x00, 0x00}},
		{name: "negative infinity", bits: 0xff800000, wire: []byte{0xca, 0xff, 0x80, 0x00, 0x00}},
		{name: "quiet NaN payload", bits: 0x7fc12345, wire: []byte{0xca, 0x7f, 0xc1, 0x23, 0x45}},
		{name: "signaling NaN payload", bits: 0x7f812345, wire: []byte{0xca, 0x7f, 0x81, 0x23, 0x45}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				writer bytes.Buffer
				e      = NewWriter(&writer)
				err    = e.EncodeFloat32(math.Float32frombits(tt.bits))
			)

			if err != nil || !bytes.Equal(writer.Bytes(), tt.wire) {
				t.Fatalf("EncodeFloat32(bits %08x) = (%x, %v), want (%x, nil)", tt.bits, writer.Bytes(), err, tt.wire)
			}
		})
	}
}

func TestWriterFloat64Wire(t *testing.T) {
	tests := []struct {
		name string
		bits uint64
		wire []byte
	}{
		{name: "positive zero", bits: 0x0000000000000000, wire: []byte{0xcb, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative zero", bits: 0x8000000000000000, wire: []byte{0xcb, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "one and a half", bits: 0x3ff8000000000000, wire: []byte{0xcb, 0x3f, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "smallest subnormal", bits: 0x0000000000000001, wire: []byte{0xcb, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}},
		{name: "maximum finite", bits: 0x7fefffffffffffff, wire: []byte{0xcb, 0x7f, 0xef, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "positive infinity", bits: 0x7ff0000000000000, wire: []byte{0xcb, 0x7f, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "negative infinity", bits: 0xfff0000000000000, wire: []byte{0xcb, 0xff, 0xf0, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "quiet NaN payload", bits: 0x7ff8123456789abc, wire: []byte{0xcb, 0x7f, 0xf8, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
		{name: "signaling NaN payload", bits: 0x7ff0123456789abc, wire: []byte{0xcb, 0x7f, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				writer bytes.Buffer
				e      = NewWriter(&writer)
				err    = e.EncodeFloat64(math.Float64frombits(tt.bits))
			)

			if err != nil || !bytes.Equal(writer.Bytes(), tt.wire) {
				t.Fatalf("EncodeFloat64(bits %016x) = (%x, %v), want (%x, nil)", tt.bits, writer.Bytes(), err, tt.wire)
			}
		})
	}
}

func TestWriterConsecutiveScalars(t *testing.T) {
	var (
		writer = &encoderTestWriter{}
		e      = NewWriter(writer)
		steps  = []struct {
			run  func() (err error)
			wire []byte
		}{
			{run: e.EncodeNil, wire: []byte{0xc0}},
			{run: func() (err error) { return e.EncodeBool(true) }, wire: []byte{0xc3}},
			{run: func() (err error) { return e.EncodeInt64(-33) }, wire: []byte{0xd0, 0xdf}},
			{run: func() (err error) { return e.EncodeUint64(128) }, wire: []byte{0xcc, 0x80}},
			{run: func() (err error) { return e.EncodeFloat32(1.5) }, wire: []byte{0xca, 0x3f, 0xc0, 0x00, 0x00}},
			{run: func() (err error) { return e.EncodeFloat64(1.5) }, wire: []byte{0xcb, 0x3f, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
			{run: func() (err error) { return e.EncodeBool(false) }, wire: []byte{0xc2}},
		}
		want []byte
	)

	for index, step := range steps {
		if err := step.run(); err != nil {
			t.Fatalf("operation %d: %v", index, err)
		}

		want = append(want, step.wire...)
		if !bytes.Equal(writer.data, want) {
			t.Fatalf("operation %d produced %x, want %x immediately", index, writer.data, want)
		}
	}
	if writer.flushes != 0 || writer.closes != 0 {
		t.Fatal("Writer flushed or closed its writer")
	}
}

func TestWriterWriterFailures(t *testing.T) {
	var (
		failure = errors.New("test destination failure")
		wire    = []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		tests   = []struct {
			name    string
			limit   int
			limited bool
			err     error
			wantN   int
			wantErr error
		}{
			{name: "error before output", limited: true, err: failure, wantErr: failure},
			{name: "partial write with error", limit: 3, limited: true, err: failure, wantN: 3, wantErr: failure},
			{name: "full write with error", err: failure, wantN: 9, wantErr: failure},
			{name: "short write without error", limit: 3, limited: true, wantN: 3, wantErr: io.ErrShortWrite},
			{name: "zero progress without error", limited: true, wantErr: io.ErrShortWrite},
		}
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				writer = &encoderTestWriter{limit: tt.limit, limited: tt.limited, err: tt.err}
				e      = NewWriter(writer)
				first  = e.EncodeUint64(math.MaxUint64)
				later  = []struct {
					name string
					run  func() (err error)
				}{
					{name: "nil", run: e.EncodeNil},
					{name: "bool", run: func() (err error) { return e.EncodeBool(true) }},
					{name: "int64", run: func() (err error) { return e.EncodeInt64(-33) }},
					{name: "uint64", run: func() (err error) { return e.EncodeUint64(128) }},
					{name: "float32", run: func() (err error) { return e.EncodeFloat32(1.5) }},
					{name: "float64", run: func() (err error) { return e.EncodeFloat64(1.5) }},
				}
			)

			if !errors.Is(first, tt.wantErr) || first == tt.wantErr {
				t.Fatalf("first error = %v, want context wrapping %v", first, tt.wantErr)
			}

			if !strings.Contains(first.Error(), "encode uint64") || !strings.Contains(first.Error(), "destination") {
				t.Fatalf("error lacks operation/destination context: %v", first)
			}

			if writer.writes != 1 || !bytes.Equal(writer.data, wire[:tt.wantN]) {
				t.Fatalf("failed write = (%d calls, %x), want (1, %x)", writer.writes, writer.data, wire[:tt.wantN])
			}

			for _, operation := range later {
				if err := operation.run(); err != first {
					t.Fatalf("later %s returned %v, want stored error %v", operation.name, err, first)
				}
			}
			if writer.writes != 1 || !bytes.Equal(writer.data, wire[:tt.wantN]) {
				t.Fatal("a terminal Writer performed additional output")
			}

			if writer.flushes != 0 || writer.closes != 0 {
				t.Fatal("failed Writer flushed or closed its writer")
			}
		})
	}
}

func TestWriterUninitialized(t *testing.T) {
	tests := []struct {
		name  string
		build func() (e *Writer)
	}{
		{name: "zero value", build: func() (e *Writer) { return &Writer{} }},
		{name: "nil constructor", build: func() (e *Writer) { return NewWriter(nil) }},
		{name: "nil reset", build: func() (e *Writer) {
			e = NewWriter(io.Discard)
			e.Reset(nil)
			return e
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				e          = tt.build()
				operations = []func() (err error){
					e.EncodeNil,
					func() (err error) { return e.EncodeBool(true) },
					func() (err error) { return e.EncodeInt64(-33) },
					func() (err error) { return e.EncodeUint64(128) },
					func() (err error) { return e.EncodeFloat32(1.5) },
					func() (err error) { return e.EncodeFloat64(1.5) },
				}
				writer encoderTestWriter
			)

			for index, operation := range operations {
				if err := operation(); err != ErrUninitialized {
					t.Fatalf("unattached operation %d returned %v, want direct ErrUninitialized", index, err)
				}
			}
			e.Reset(&writer)
			if err := e.EncodeBool(true); err != nil || !bytes.Equal(writer.data, []byte{0xc3}) {
				t.Fatalf("reset/encode returned (%x, %v), want (c3, nil)", writer.data, err)
			}
		})
	}
}

func TestWriterReset(t *testing.T) {
	var (
		failure   = errors.New("test destination failure")
		oldWriter = &encoderTestWriter{limit: 1, limited: true, err: failure}
		newWriter encoderTestWriter
		e         = NewWriter(oldWriter)
		first     = e.EncodeUint64(128)
	)

	if !errors.Is(first, failure) {
		t.Fatalf("initial encode error = %v, want %v", first, failure)
	}

	e.Reset(&newWriter)
	if err := e.EncodeBool(true); err != nil {
		t.Fatal(err)
	}

	e.Reset(&newWriter)
	if err := e.EncodeNil(); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(newWriter.data, []byte{0xc3, 0xc0}) {
		t.Fatalf("reset output = %x, want c3c0", newWriter.data)
	}

	if oldWriter.writes != 1 || !bytes.Equal(oldWriter.data, []byte{0xcc}) {
		t.Fatalf("reset repaired or reused partial old output: %x", oldWriter.data)
	}

	e.Reset(nil)
	if err := e.EncodeNil(); err != ErrUninitialized {
		t.Fatalf("nil reset returned %v, want ErrUninitialized", err)
	}

	if oldWriter.closes != 0 || oldWriter.flushes != 0 || newWriter.closes != 0 || newWriter.flushes != 0 {
		t.Fatal("reset flushed or closed caller-owned writers")
	}
}

func TestWriterTypedNilWriter(t *testing.T) {
	var (
		writer *encoderTestWriter
		e      = NewWriter(writer)
		err    = e.EncodeNil()
	)

	if err != nil {
		t.Fatalf("typed nil writer with a valid Write method returned %v", err)
	}
}

func TestWriterCopiesShareFailureAndReset(t *testing.T) {
	var (
		failure = errors.New("test: destination failure")
		output  = &encoderTestWriter{limit: 1, limited: true, err: failure}
		w       = NewWriter(output)
		copied  = *w
		first   = copied.EncodeUint64(128)
		next    bytes.Buffer
		err     error
	)

	if !errors.Is(first, failure) {
		t.Fatalf("copied Writer error = %v, want %v", first, failure)
	}

	if err = w.EncodeNil(); err != first || output.writes != 1 {
		t.Fatalf("original Writer error = %v, writes %d, want stored error without I/O", err, output.writes)
	}

	copied.Reset(&next)
	if err = w.EncodeBool(true); err != nil {
		t.Fatal(err)
	}

	if err = copied.EncodeNil(); err != nil || !bytes.Equal(next.Bytes(), []byte{0xc3, 0xc0}) {
		t.Fatalf("shared reset output = (%x, %v), want (c3c0, nil)", next.Bytes(), err)
	}

	w.Reset(nil)
	if err = copied.EncodeNil(); err != ErrUninitialized {
		t.Fatalf("shared nil Reset error = %v, want ErrUninitialized", err)
	}

	if output.writes != 1 || output.flushes != 0 || output.closes != 0 {
		t.Fatal("shared Reset reused, flushed, or closed the previous output")
	}
}

func TestWriterInt64WireFixtures(t *testing.T) {
	type testcase struct {
		name  string
		value int64
		want  []byte
	}
	tests := []testcase{
		{name: "minimum int64", value: math.MinInt64, want: []byte{0xd3, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "below int32", value: -2147483649, want: []byte{0xd3, 0xff, 0xff, 0xff, 0xff, 0x7f, 0xff, 0xff, 0xff}},
		{name: "minimum int32", value: -2147483648, want: []byte{0xd2, 0x80, 0x00, 0x00, 0x00}},
		{name: "below int16", value: -32769, want: []byte{0xd2, 0xff, 0xff, 0x7f, 0xff}},
		{name: "minimum int16", value: -32768, want: []byte{0xd1, 0x80, 0x00}},
		{name: "below int8", value: -129, want: []byte{0xd1, 0xff, 0x7f}},
		{name: "minimum int8", value: -128, want: []byte{0xd0, 0x80}},
		{name: "below negative fixint", value: -33, want: []byte{0xd0, 0xdf}},
		{name: "minimum negative fixint", value: -32, want: []byte{0xe0}},
		{name: "negative one", value: -1, want: []byte{0xff}},
		{name: "zero", value: 0, want: []byte{0x00}},
		{name: "one", value: 1, want: []byte{0x01}},
		{name: "maximum positive fixint", value: 127, want: []byte{0x7f}},
		{name: "above positive fixint", value: 128, want: []byte{0xcc, 0x80}},
		{name: "maximum uint8", value: 255, want: []byte{0xcc, 0xff}},
		{name: "above uint8", value: 256, want: []byte{0xcd, 0x01, 0x00}},
		{name: "maximum uint16", value: 65535, want: []byte{0xcd, 0xff, 0xff}},
		{name: "above uint16", value: 65536, want: []byte{0xce, 0x00, 0x01, 0x00, 0x00}},
		{name: "maximum uint32", value: 4294967295, want: []byte{0xce, 0xff, 0xff, 0xff, 0xff}},
		{name: "above uint32", value: 4294967296, want: []byte{0xcf, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}},
		{name: "maximum int64", value: math.MaxInt64, want: []byte{0xcf, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				output bytes.Buffer
				w      = NewWriter(&output)
				err    = w.EncodeInt64(tt.value)
			)

			if err != nil || !bytes.Equal(output.Bytes(), tt.want) {
				t.Fatalf("EncodeInt64(%d) = (%x, %v), want %x", tt.value, output.Bytes(), err, tt.want)
			}
		})
	}
}

func TestWriterUint64WireFixtures(t *testing.T) {
	type testcase struct {
		name  string
		value uint64
		want  []byte
	}
	tests := []testcase{
		{name: "zero", value: 0, want: []byte{0x00}},
		{name: "one", value: 1, want: []byte{0x01}},
		{name: "maximum positive fixint", value: 127, want: []byte{0x7f}},
		{name: "above positive fixint", value: 128, want: []byte{0xcc, 0x80}},
		{name: "maximum uint8", value: 255, want: []byte{0xcc, 0xff}},
		{name: "above uint8", value: 256, want: []byte{0xcd, 0x01, 0x00}},
		{name: "maximum uint16", value: 65535, want: []byte{0xcd, 0xff, 0xff}},
		{name: "above uint16", value: 65536, want: []byte{0xce, 0x00, 0x01, 0x00, 0x00}},
		{name: "maximum uint32", value: 4294967295, want: []byte{0xce, 0xff, 0xff, 0xff, 0xff}},
		{name: "above uint32", value: 4294967296, want: []byte{0xcf, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00}},
		{name: "maximum int64", value: math.MaxInt64, want: []byte{0xcf, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
		{name: "above int64", value: uint64(math.MaxInt64) + 1, want: []byte{0xcf, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}},
		{name: "maximum uint64", value: math.MaxUint64, want: []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				output bytes.Buffer
				w      = NewWriter(&output)
				err    = w.EncodeUint64(tt.value)
			)

			if err != nil || !bytes.Equal(output.Bytes(), tt.want) {
				t.Fatalf("EncodeUint64(%d) = (%x, %v), want %x", tt.value, output.Bytes(), err, tt.want)
			}
		})
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

func TestWriterZeroAllocations(t *testing.T) {
	var (
		w          = NewWriter(io.Discard)
		binaryWire = []byte{0xc4, 8, 'm', 's', 'g', 'p', 'a', 'c', 'k', '!'}
	)

	if err := w.EncodeString("warm"); err != nil {
		t.Fatal(err)
	}

	allocations := testing.AllocsPerRun(1000, func() {
		var err error
		if err = w.EncodeNil(); err != nil {
			t.Fatal(err)
		}

		if err = w.EncodeBool(true); err != nil {
			t.Fatal(err)
		}

		if err = w.EncodeInt64(-32769); err != nil {
			t.Fatal(err)
		}

		if err = w.EncodeUint64(^uint64(0)); err != nil {
			t.Fatal(err)
		}

		if err = w.EncodeFloat32(1.5); err != nil {
			t.Fatal(err)
		}

		if err = w.EncodeFloat64(1.5); err != nil {
			t.Fatal(err)
		}

		if err = w.EncodeArrayHeader(65536); err != nil {
			t.Fatal(err)
		}

		if err = w.EncodeMapHeader(65536); err != nil {
			t.Fatal(err)
		}

		if err = w.EncodeString("msgpack!"); err != nil {
			t.Fatal(err)
		}

		if err = w.EncodeBytes(binaryWire[2:]); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("reused Writer allocated %g times per run, want zero", allocations)
	}
}

func TestWriterShortStringAllocations(t *testing.T) {
	for _, value := range []string{"", "x", "12345678", "123456789"} {
		t.Run(fmt.Sprintf("length_%d", len(value)), func(t *testing.T) {
			allocations := testing.AllocsPerRun(1000, func() {
				var w = NewWriter(io.Discard)
				if err := w.EncodeString(value); err != nil {
					t.Fatal(err)
				}
			})
			if allocations > 2 {
				t.Fatalf("fresh short string encoding allocated %g times, want at most two codec allocations", allocations)
			}
		})
	}
}

func TestWriterPayloadWire(t *testing.T) {
	tests := []struct {
		name         string
		length       int
		stringHeader []byte
		binaryHeader []byte
	}{
		{name: "empty", stringHeader: []byte{0xa0}, binaryHeader: []byte{0xc4, 0}},
		{name: "one", length: 1, stringHeader: []byte{0xa1}, binaryHeader: []byte{0xc4, 1}},
		{name: "8", length: 8, stringHeader: []byte{0xa8}, binaryHeader: []byte{0xc4, 8}},
		{name: "9", length: 9, stringHeader: []byte{0xa9}, binaryHeader: []byte{0xc4, 9}},
		{name: "10", length: 10, stringHeader: []byte{0xaa}, binaryHeader: []byte{0xc4, 10}},
		{name: "31", length: 31, stringHeader: []byte{0xbf}, binaryHeader: []byte{0xc4, 31}},
		{name: "32", length: 32, stringHeader: []byte{0xd9, 32}, binaryHeader: []byte{0xc4, 32}},
		{name: "255", length: 255, stringHeader: []byte{0xd9, 0xff}, binaryHeader: []byte{0xc4, 0xff}},
		{name: "256", length: 256, stringHeader: []byte{0xda, 1, 0}, binaryHeader: []byte{0xc5, 1, 0}},
		{name: "65535", length: 65535, stringHeader: []byte{0xda, 0xff, 0xff}, binaryHeader: []byte{0xc5, 0xff, 0xff}},
		{name: "65536", length: 65536, stringHeader: []byte{0xdb, 0, 1, 0, 0}, binaryHeader: []byte{0xc6, 0, 1, 0, 0}},
	}
	for _, tt := range tests {
		value := bytes.Repeat([]byte{0x78}, tt.length)
		methods := []struct {
			name   string
			header []byte
			run    func(e *Writer) (err error)
		}{
			{name: "string", header: tt.stringHeader, run: func(e *Writer) (err error) {
				return e.EncodeString(string(value))
			}},
			{name: "bytes", header: tt.binaryHeader, run: func(e *Writer) (err error) {
				return e.EncodeBytes(value)
			}},
			{name: "string from", header: tt.stringHeader, run: func(e *Writer) (err error) {
				n, sourceErr := e.EncodeStringFrom(uint64(len(value)), bytes.NewReader(value))
				if n != uint64(len(value)) {
					return fmt.Errorf("string payload progress = %d, want %d", n, len(value))
				}

				return sourceErr
			}},
			{name: "bytes from", header: tt.binaryHeader, run: func(e *Writer) (err error) {
				n, sourceErr := e.EncodeBytesFrom(uint64(len(value)), bytes.NewReader(value))
				if n != uint64(len(value)) {
					return fmt.Errorf("binary payload progress = %d, want %d", n, len(value))
				}

				return sourceErr
			}},
		}
		for _, method := range methods {
			t.Run(tt.name+"/"+method.name, func(t *testing.T) {
				var (
					writer = &encoderPayloadWriter{}
					e      = NewWriter(writer)
					want   = append(append([]byte(nil), method.header...), value...)
					err    = method.run(e)
				)

				if err != nil || !bytes.Equal(writer.data, want) {
					t.Fatalf("encoding length %d = %d bytes, %v; want %d exact bytes", tt.length, len(writer.data), err, len(want))
				}

				if writer.maxRequest > 32768 || writer.flushes != 0 || writer.closes != 0 {
					t.Fatal("payload write exceeded chunk bound or changed writer ownership")
				}
			})
		}
	}
}

func TestWriterPayloadRawBytesAndConsecutiveValues(t *testing.T) {
	var (
		writer bytes.Buffer
		e      = NewWriter(&writer)
		value  = []byte{0, 0xc3, 0xa9, 0xff}
		want   = []byte{
			0xa4, 0, 0xc3, 0xa9, 0xff, 0xc4, 4, 0, 0xc3, 0xa9, 0xff,
			0xa4, 0, 0xc3, 0xa9, 0xff, 0xc4, 4, 0, 0xc3, 0xa9, 0xff,
			0xc4, 0, 0xc0, 0xc3,
		}
	)

	if err := e.EncodeString(string(value)); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeBytes(value); err != nil {
		t.Fatal(err)
	}

	if n, err := e.EncodeStringFrom(4, bytes.NewReader(value)); n != 4 || err != nil {
		t.Fatalf("raw string transfer = (%d, %v)", n, err)
	}

	if n, err := e.EncodeBytesFrom(4, bytes.NewReader(value)); n != 4 || err != nil {
		t.Fatalf("raw binary transfer = (%d, %v)", n, err)
	}

	if err := e.EncodeBytes(nil); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeNil(); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeBool(true); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(writer.Bytes(), want) {
		t.Fatalf("raw string/binary and nil = %x, want %x", writer.Bytes(), want)
	}
}

func TestWriterFromSourceBoundaries(t *testing.T) {
	methods := []struct {
		name string
		run  func(e *Writer, length uint64, src io.Reader) (n uint64, err error)
	}{
		{name: "string", run: (*Writer).EncodeStringFrom},
		{name: "bytes", run: (*Writer).EncodeBytesFrom},
	}
	tests := []struct {
		name   string
		length uint64
		data   []byte
		end    error
		wantN  uint64
		want   error
	}{
		{name: "empty source", length: 3, want: io.ErrUnexpectedEOF},
		{name: "short source", length: 3, data: []byte{1, 2}, wantN: 2, want: io.ErrUnexpectedEOF},
		{name: "short data and EOF", length: 3, data: []byte{1, 2}, end: io.EOF, wantN: 2, want: io.ErrUnexpectedEOF},
		{name: "exact source", length: 3, data: []byte{1, 2, 3}, wantN: 3},
		{name: "exact data and EOF", length: 3, data: []byte{1, 2, 3}, end: io.EOF, wantN: 3},
		{name: "extra source byte", length: 3, data: []byte{1, 2, 3, 9}, wantN: 3},
		{name: "zero ignores source", data: []byte{9}, end: io.ErrClosedPipe},
	}
	for _, method := range methods {
		for _, tt := range tests {
			t.Run(method.name+"/"+tt.name, func(t *testing.T) {
				var (
					source = &decoderTestReader{data: tt.data, maxRead: 1, endErr: tt.end}
					writer = &encoderPayloadWriter{}
					e      = NewWriter(writer)
					n, err = method.run(e, tt.length, source)
				)

				if n != tt.wantN || !errors.Is(err, tt.want) || uint64(source.position) != tt.wantN {
					t.Fatalf("transfer = (%d, %v), consumed %d; want (%d, %v)", n, err, source.position, tt.wantN, tt.want)
				}

				if tt.length == 0 && source.reads != 0 {
					t.Fatal("zero-length transfer read its source")
				}

				if tt.want != nil {
					reads, writes := source.reads, writer.writes
					if later := e.EncodeBool(true); later != err || source.reads != reads || writer.writes != writes {
						t.Fatal("source failure did not stop later I/O")
					}

					if errors.Is(err, io.EOF) || errors.Is(err, errFuzzTruncated) || !strings.Contains(err.Error(), "source") {
						t.Fatalf("source EOF has wrong identity or context: %v", err)
					}
				}
			})
		}
	}
}

func TestWriterFromValidationAndReset(t *testing.T) {
	methods := []struct {
		name string
		run  func(e *Writer, length uint64, src io.Reader) (n uint64, err error)
	}{
		{name: "string", run: (*Writer).EncodeStringFrom},
		{name: "bytes", run: (*Writer).EncodeBytesFrom},
	}
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			var (
				writer = &encoderPayloadWriter{}
				source = &decoderTestReader{data: []byte{1}}
				e      = NewWriter(writer)
			)

			for _, length := range []uint64{4294967296, math.MaxUint64} {
				for _, input := range []io.Reader{nil, source} {
					n, err := method.run(e, length, input)
					if n != 0 || err != ErrOverflow || writer.writes != 0 || source.reads != 0 {
						t.Fatalf("oversized length = (%d, %v), writes %d, reads %d", n, err, writer.writes, source.reads)
					}
				}
			}
			var n, err = method.run(e, 1, nil)
			if n != 0 || err != ErrInvalidArgument || writer.writes != 0 {
				t.Fatalf("nil source = (%d, %v), writes %d", n, err, writer.writes)
			}

			n, err = method.run(e, 0, nil)
			if n != 0 || err != nil {
				t.Fatalf("zero nil source = (%d, %v)", n, err)
			}

			n, err = method.run(e, 1, source)
			if n != 1 || err != nil {
				t.Fatalf("validation was terminal: (%d, %v)", n, err)
			}

			e.Reset(nil)
			n, err = method.run(e, math.MaxUint64, nil)
			if n != 0 || err != ErrUninitialized || e.EncodeString("") != ErrUninitialized || e.EncodeBytes(nil) != ErrUninitialized {
				t.Fatalf("unattached argument precedence = (%d, %v)", n, err)
			}

			e.Reset(writer)
			n, err = method.run(e, 4294967295, bytes.NewReader(nil))
			if n != 0 || !errors.Is(err, io.ErrUnexpectedEOF) {
				t.Fatalf("maximum valid length with empty source = (%d, %v)", n, err)
			}

			first := err
			n, err = method.run(e, math.MaxUint64, nil)
			if n != 0 || err != first || e.EncodeString("") != first || e.EncodeBytes(nil) != first {
				t.Fatal("terminal error lost precedence to validation or payload calls")
			}

			e.Reset(writer)
			n, err = method.run(e, 0, nil)
			if n != 0 || err != nil || writer.flushes != 0 || writer.closes != 0 {
				t.Fatalf("Reset recovery/ownership = (%d, %v)", n, err)
			}

			var typedNil *decoderTestReader
			n, err = method.run(e, 1, typedNil)
			if n != 1 || err != nil {
				t.Fatalf("typed-nil source did not retain ordinary method behavior: (%d, %v)", n, err)
			}
		})
	}
}

func TestWriterFromReadFailures(t *testing.T) {
	failure := errors.New("payload source failure")
	tests := []struct {
		name    string
		data    []byte
		end     error
		empty   int
		between int
		wantN   uint64
		want    error
	}{
		{name: "before data", end: failure, want: failure},
		{name: "partial data with error", data: []byte{1, 2}, end: failure, wantN: 2, want: failure},
		{name: "full data with error", data: []byte{1, 2, 3}, end: failure, wantN: 3, want: failure},
		{name: "99 empty reads then progress", data: []byte{1, 2, 3}, empty: 99, between: 99, wantN: 3},
		{name: "100 empty reads", empty: 100, want: io.ErrNoProgress},
		{name: "progress then 100 empty reads", data: []byte{1, 2, 3}, between: 100, wantN: 1, want: io.ErrNoProgress},
	}
	for _, isString := range []bool{false, true} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("string_%t/%s", isString, tt.name), func(t *testing.T) {
				var (
					source = &decoderTestReader{data: tt.data, endErr: tt.end, maxRead: 1, initialEmpty: tt.empty, emptyBetween: tt.between}
					writer = &encoderPayloadWriter{}
					e      = NewWriter(writer)
					n      uint64
					err    error
				)

				if isString {
					n, err = e.EncodeStringFrom(3, source)
				} else {
					n, err = e.EncodeBytesFrom(3, source)
				}

				if n != tt.wantN || !errors.Is(err, tt.want) {
					t.Fatalf("source fault = (%d, %v), want (%d, %v)", n, err, tt.wantN, tt.want)
				}

				if tt.want != nil && (err == tt.want || !strings.Contains(err.Error(), "source") || e.EncodeNil() != err) {
					t.Fatalf("source fault lacks context/sticky identity: %v", err)
				}
			})
		}
	}
}

func TestWriterFromWriteFailures(t *testing.T) {
	var (
		sourceFailure = errors.New("simultaneous source failure")
		writeFailure  = errors.New("payload destination failure")
		tests         = []struct {
			name      string
			call      int
			accept    int
			acceptAll bool
			writeErr  error
			sourceErr error
			wantN     uint64
			want      error
			wantReads int
		}{
			{name: "header no output", call: 1, writeErr: writeFailure, want: writeFailure},
			{name: "partial header", call: 1, accept: 1, writeErr: writeFailure, want: writeFailure},
			{name: "full header with error", call: 1, acceptAll: true, writeErr: writeFailure, want: writeFailure},
			{name: "header short nil error", call: 1, accept: 1, want: io.ErrShortWrite},
			{name: "payload no output", call: 2, writeErr: writeFailure, want: writeFailure, wantReads: 1},
			{name: "payload partial error", call: 2, accept: 2, writeErr: writeFailure, wantN: 2, want: writeFailure, wantReads: 1},
			{name: "payload partial nil error", call: 2, accept: 2, wantN: 2, want: io.ErrShortWrite, wantReads: 1},
			{name: "payload zero progress", call: 2, want: io.ErrShortWrite, wantReads: 1},
			{name: "payload full with error", call: 2, acceptAll: true, writeErr: writeFailure, wantN: 3, want: writeFailure, wantReads: 1},
			{name: "both ends fail", call: 2, accept: 2, writeErr: writeFailure, sourceErr: sourceFailure, wantN: 2, want: writeFailure, wantReads: 1},
		}
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				source = &decoderTestReader{data: []byte{1, 2, 3}, endErr: tt.sourceErr}
				writer = &encoderPayloadWriter{failCall: tt.call, accept: tt.accept, acceptAll: tt.acceptAll, err: tt.writeErr}
				e      = NewWriter(writer)
				n, err = e.EncodeBytesFrom(3, source)
			)

			if n != tt.wantN || !errors.Is(err, tt.want) || source.reads != tt.wantReads {
				t.Fatalf("write fault = (%d, %v), reads %d; want (%d, %v), reads %d", n, err, source.reads, tt.wantN, tt.want, tt.wantReads)
			}

			if !strings.Contains(err.Error(), "destination") || errors.Is(err, sourceFailure) {
				t.Fatalf("wrong simultaneous fault precedence/context: %v", err)
			}

			reads, writes := source.reads, writer.writes
			laterN, laterErr := e.EncodeStringFrom(1, source)
			if laterN != 0 || laterErr != err || reads != source.reads || writes != writer.writes {
				t.Fatal("destination failure did not prevent future source/output I/O")
			}
		})
	}
}

func TestWriterShortStringFailures(t *testing.T) {
	var failure = errors.New("short string write failure")
	tests := []struct {
		name      string
		failCall  int
		accept    int
		acceptAll bool
		err       error
		want      error
	}{
		{name: "header error", failCall: 1, err: failure, want: failure},
		{name: "header short write", failCall: 1, want: io.ErrShortWrite},
		{name: "payload error", failCall: 2, accept: 3, err: failure, want: failure},
		{name: "payload short write", failCall: 2, accept: 3, want: io.ErrShortWrite},
		{name: "complete payload with error", failCall: 2, acceptAll: true, err: failure, want: failure},
	}
	for _, length := range []int{9, 10} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("length_%d/%s", length, tt.name), func(t *testing.T) {
				var (
					value  = strings.Repeat("x", length)
					writer = encoderPayloadWriter{failCall: tt.failCall, accept: tt.accept, acceptAll: tt.acceptAll, err: tt.err}
					w      = NewWriter(&writer)
					err    = w.EncodeString(value)
					want   []byte
				)

				if !errors.Is(err, tt.want) || writer.writes != tt.failCall {
					t.Fatalf("EncodeString = %v after %d writes, want %v after %d", err, writer.writes, tt.want, tt.failCall)
				}

				if tt.failCall == 2 {
					want = append(want, 0xa0|byte(length))
					if tt.acceptAll {
						want = append(want, value...)
					} else {
						want = append(want, value[:tt.accept]...)
					}
				}

				if !bytes.Equal(writer.data, want) {
					t.Fatalf("accepted wire = %x, want %x", writer.data, want)
				}

				if laterErr := w.EncodeString(value); laterErr != err || writer.writes != tt.failCall {
					t.Fatal("stored string failure did not prevent later I/O")
				}
			})
		}
	}
}

func TestWriterInMemoryPayloadFailures(t *testing.T) {
	for _, isString := range []bool{false, true} {
		for _, call := range []int{1, 2, 3} {
			t.Run(fmt.Sprintf("string_%t/call_%d", isString, call), func(t *testing.T) {
				var (
					failure = errors.New("in-memory payload write failure")
					writer  = &encoderPayloadWriter{failCall: call, accept: 1, err: failure}
					e       = NewWriter(writer)
					value   = bytes.Repeat([]byte{0xff}, 65536)
					err     error
				)

				if isString {
					err = e.EncodeString(string(value))
				} else {
					err = e.EncodeBytes(value)
				}

				if !errors.Is(err, failure) || writer.writes != call || writer.maxRequest > 32768 || e.EncodeNil() != err {
					t.Fatalf("in-memory payload fault = %v, writes %d, maximum request %d", err, writer.writes, writer.maxRequest)
				}
			})
		}
	}
}

func TestWriterFromLaterChunkFailures(t *testing.T) {
	var (
		sourceFailure = errors.New("later source failure")
		writeFailure  = errors.New("later destination failure")
		tests         = []struct {
			name         string
			sourceLength int
			sourceErr    error
			failCall     int
			accept       int
			writeErr     error
			wantN        uint64
			want         error
		}{
			{name: "second source chunk fails", sourceLength: 32775, sourceErr: sourceFailure, wantN: 32775, want: sourceFailure},
			{name: "second destination chunk fails", sourceLength: 70001, failCall: 3, accept: 7, writeErr: writeFailure, wantN: 32775, want: writeFailure},
			{name: "second destination chunk short", sourceLength: 70001, failCall: 3, accept: 7, wantN: 32775, want: io.ErrShortWrite},
			{name: "simultaneous later failures", sourceLength: 32775, sourceErr: sourceFailure, failCall: 3, accept: 3, writeErr: writeFailure, wantN: 32771, want: writeFailure},
		}
	)

	for _, isString := range []bool{false, true} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("string_%t/%s", isString, tt.name), func(t *testing.T) {
				var (
					source = &decoderTestReader{data: bytes.Repeat([]byte{0x78}, tt.sourceLength), endErr: tt.sourceErr}
					writer = &encoderPayloadWriter{failCall: tt.failCall, accept: tt.accept, err: tt.writeErr}
					e      = NewWriter(writer)
					n      uint64
					err    error
				)

				if isString {
					n, err = e.EncodeStringFrom(70000, source)
				} else {
					n, err = e.EncodeBytesFrom(70000, source)
				}

				if n != tt.wantN || !errors.Is(err, tt.want) || source.reads != 2 || writer.writes != 3 {
					t.Fatalf("later chunk failure = (%d, %v), reads %d, writes %d", n, err, source.reads, writer.writes)
				}

				if len(writer.data) != 5+int(n) || uint64(source.position)-n > 32768 {
					t.Fatalf("header included in count or source over-consumed: n %d, written %d, read %d", n, len(writer.data), source.position)
				}

				if e.EncodeBytes(nil) != err || source.reads != 2 || writer.writes != 3 {
					t.Fatal("later failure did not stop all future I/O")
				}
			})
		}
	}
}

func TestWriterGeneratedPayloadIsIncremental(t *testing.T) {
	const length = 8*1024*1024 + 7
	for _, isString := range []bool{false, true} {
		t.Run(fmt.Sprintf("string_%t", isString), func(t *testing.T) {
			var (
				source = &encoderGeneratedReader{length: length}
				header = []byte{0xc6, 0, 0x80, 0, 7}
				n      uint64
				err    error
			)

			if isString {
				header[0] = 0xdb
			}

			writer := &encoderCountingWriter{header: header, source: source}
			e := NewWriter(writer)
			if isString {
				n, err = e.EncodeStringFrom(length, source)
			} else {
				n, err = e.EncodeBytesFrom(length, source)
			}

			if err != nil || n != length || source.read != length || writer.bytes != length || writer.invalid {
				t.Fatalf("generated transfer = (%d, %v), read %d, wrote %d, invalid %t", n, err, source.read, writer.bytes, writer.invalid)
			}

			if source.maxRequest > 32768 || writer.maxRequest > 32768 || writer.firstPayloadSourceRead >= length {
				t.Fatalf("unbounded/staged transfer: source request %d, write request %d, first output after %d source bytes", source.maxRequest, writer.maxRequest, writer.firstPayloadSourceRead)
			}
		})
	}
}
