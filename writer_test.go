package messagepack

import (
	"bytes"
	"errors"
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
