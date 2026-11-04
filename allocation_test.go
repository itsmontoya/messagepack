package messagepack

import (
	"bytes"
	"io"
	"testing"
)

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

func TestReaderZeroAllocations(t *testing.T) {
	var (
		wire = []byte{0xc0, 0xc3, 0xd2, 0xff, 0xff, 0x7f, 0xff,
			0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
			0xca, 0x3f, 0xc0, 0, 0, 0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0,
			0xdd, 0, 1, 0, 0, 0xdf, 0, 1, 0, 0}
		input bytes.Reader
		r     = NewReader(&input)
	)
	allocations := testing.AllocsPerRun(1000, func() {
		input.Reset(wire)
		r.Reset(&input)
		var err error
		if err = r.DecodeNil(); err != nil {
			t.Fatal(err)
		}
		if benchmarkBoolSink, err = r.DecodeBool(); err != nil {
			t.Fatal(err)
		}
		if benchmarkIntSink, err = r.DecodeInt64(); err != nil {
			t.Fatal(err)
		}
		if benchmarkUintSink, err = r.DecodeUint64(); err != nil {
			t.Fatal(err)
		}
		if benchmarkFloat32Sink, err = r.DecodeFloat32(); err != nil {
			t.Fatal(err)
		}
		if benchmarkFloat64Sink, err = r.DecodeFloat64(); err != nil {
			t.Fatal(err)
		}
		if benchmarkHeaderSink, err = r.DecodeArrayHeader(); err != nil {
			t.Fatal(err)
		}
		if benchmarkHeaderSink, err = r.DecodeMapHeader(); err != nil {
			t.Fatal(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("reused Reader allocated %g times per run, want zero", allocations)
	}
}
