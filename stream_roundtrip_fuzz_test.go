package messagepack

import (
	"bytes"
	"io"
	"math"
	"testing"
)

func FuzzStreamRoundTrip(f *testing.F) {
	for _, signed := range []int64{math.MinInt64, -2147483649, -2147483648, -32769, -32768,
		-129, -128, -33, -32, -1, 0, 127, 128, 255, 256, 65535, 65536, 4294967295, 4294967296, math.MaxInt64} {
		f.Add(signed, uint64(signed), uint32(signed), uint64(signed), []byte{0, 0xff, 0xc3}, []byte{0, 2, 63})
	}
	for _, bits := range []uint64{0, 1, 0x8000000000000000, 0x7ff0000000000000,
		0xfff0000000000000, 0x7ff8123456789abc, 0x7ff0123456789abc} {
		for _, narrow := range []uint32{0, 1, 0x80000000, 0x7f800000, 0xff800000, 0x7fc12345, 0x7f812345} {
			f.Add(int64(1), uint64(math.MaxUint64), narrow, bits, []byte{}, []byte{0})
		}
	}
	for _, length := range []int{1, 31, 32, 255, 256, 4096} {
		f.Add(int64(-1), uint64(4294967296), uint32(0x80000001), uint64(0x8000000000000001), bytes.Repeat([]byte{0xff}, length), []byte{0, 63})
	}
	f.Fuzz(func(t *testing.T, signed int64, unsigned uint64, bits32 uint32, bits64 uint64, payload, fragments []byte) {
		if len(payload) > streamFuzzInputLimit || len(fragments) > 32 {
			t.Skip()
		}

		var (
			before = append([]byte(nil), payload...)
			w      streamFuzzWriter
			e      = NewWriter(&w)
		)
		encodeStreamFuzzScalars(t, e, signed, unsigned, bits32, bits64)
		encodeStreamFuzzPayloads(t, e, payload, fragments)
		if !bytes.Equal(payload, before) || len(e.state.buffer) > 32768 {
			t.Fatal("typed stream encoding changed its source or exceeded bounded scratch")
		}

		want := append([]byte(nil), w.data...)
		var fresh streamFuzzWriter
		e.Reset(&fresh)
		encodeStreamFuzzScalars(t, e, signed, unsigned, bits32, bits64)
		encodeStreamFuzzPayloads(t, e, payload, fragments)
		if !bytes.Equal(fresh.data, want) || !bytes.Equal(w.data, want) {
			t.Fatal("encoder reuse changed output or retained the old destination")
		}

		r := &streamFuzzReader{data: w.data, fragments: fragments, emptyBetween: 1, endErr: io.EOF, dataError: true}
		d := NewReader(r)
		if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: 4096, MaxSkipDepth: 4, MaxSkipValues: 32, MaxSkipBytes: 20000}); err != nil {
			t.Fatal(err)
		}

		checkStreamFuzzScalars(t, d, signed, unsigned, bits32, bits64)
		checkStreamFuzzPayloads(t, d, r, payload, len(w.data))
	})
}

func encodeStreamFuzzScalars(t *testing.T, e *Writer, signed int64, unsigned uint64, bits32 uint32, bits64 uint64) {
	t.Helper()
	if err := e.EncodeArrayHeader(9); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeNil(); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeBool(signed&1 != 0); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeInt64(signed); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeUint64(unsigned); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeFloat32(math.Float32frombits(bits32)); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeFloat64(math.Float64frombits(bits64)); err != nil {
		t.Fatal(err)
	}
}

func encodeStreamFuzzPayloads(t *testing.T, e *Writer, payload, fragments []byte) {
	t.Helper()
	if err := e.EncodeString(string(payload)); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeBytes(payload); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeMapHeader(1); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeString("payload"); err != nil {
		t.Fatal(err)
	}

	if err := e.EncodeArrayHeader(2); err != nil {
		t.Fatal(err)
	}

	var (
		source = append(append([]byte(nil), payload...), 0x7e)
		reader = &streamFuzzReader{data: source, fragments: fragments, emptyBetween: 1}
	)
	if n, err := e.EncodeStringFrom(uint64(len(payload)), reader); err != nil || n != uint64(len(payload)) || reader.position != len(payload) {
		t.Fatalf("string transfer boundary = (%d, %v), position %d", n, err, reader.position)
	}

	reader = &streamFuzzReader{data: source, fragments: fragments, emptyBetween: 1}
	if n, err := e.EncodeBytesFrom(uint64(len(payload)), reader); err != nil || n != uint64(len(payload)) || reader.position != len(payload) {
		t.Fatalf("binary transfer boundary = (%d, %v), position %d", n, err, reader.position)
	}

	if err := e.EncodeBool(true); err != nil {
		t.Fatal(err)
	}
}

func checkStreamFuzzScalars(t *testing.T, d *Reader, signed int64, unsigned uint64, bits32 uint32, bits64 uint64) {
	t.Helper()
	if count, err := d.DecodeArrayHeader(); count != 9 || err != nil {
		t.Fatalf("root count = (%d, %v)", count, err)
	}

	if err := d.DecodeNil(); err != nil {
		t.Fatal(err)
	}

	if value, err := d.DecodeBool(); value != (signed&1 != 0) || err != nil {
		t.Fatalf("bool round trip = (%v, %v)", value, err)
	}

	if value, err := d.DecodeInt64(); value != signed || err != nil {
		t.Fatalf("int64 round trip = (%d, %v)", value, err)
	}

	if value, err := d.DecodeUint64(); value != unsigned || err != nil {
		t.Fatalf("uint64 round trip = (%d, %v)", value, err)
	}

	if value, err := d.DecodeFloat32(); math.Float32bits(value) != bits32 || err != nil {
		t.Fatalf("float32 round trip bits = (%x, %v), want %x", math.Float32bits(value), err, bits32)
	}

	if value, err := d.DecodeFloat64(); math.Float64bits(value) != bits64 || err != nil {
		t.Fatalf("float64 round trip bits = (%x, %v), want %x", math.Float64bits(value), err, bits64)
	}
}

func checkStreamFuzzPayloads(t *testing.T, d *Reader, r *streamFuzzReader, payload []byte, wireLength int) {
	t.Helper()
	ownedText, textErr := d.DecodeString()
	ownedBytes, bytesErr := d.DecodeBytes()
	if textErr != nil || bytesErr != nil || ownedText != string(payload) || ownedBytes == nil || !bytes.Equal(ownedBytes, payload) {
		t.Fatalf("owned payload round trip: string error %v, binary error %v", textErr, bytesErr)
	}

	if count, err := d.DecodeMapHeader(); count != 1 || err != nil {
		t.Fatalf("nested map count = (%d, %v)", count, err)
	}

	if key, err := d.DecodeString(); key != "payload" || err != nil {
		t.Fatalf("nested map key = (%q, %v)", key, err)
	}

	if count, err := d.DecodeArrayHeader(); count != 2 || err != nil {
		t.Fatalf("nested array count = (%d, %v)", count, err)
	}

	var textSink, binarySink streamFuzzWriter
	if n, err := d.DecodeStringTo(&textSink); err != nil || n != uint64(len(payload)) || !bytes.Equal(textSink.data, payload) {
		t.Fatalf("string transfer round trip = (%d, %v)", n, err)
	}

	if n, err := d.DecodeBytesTo(&binarySink); err != nil || n != uint64(len(payload)) || !bytes.Equal(binarySink.data, payload) {
		t.Fatalf("binary transfer round trip = (%d, %v)", n, err)
	}

	if r.position != wireLength-1 {
		t.Fatal("root decode crossed into the following value")
	}

	if value, err := d.DecodeBool(); !value || err != nil || r.position != wireLength {
		t.Fatalf("following sentinel = (%v, %v), position %d", value, err, r.position)
	}

	reads := r.reads
	requireStreamFuzzError(t, d.DecodeNil(), io.EOF)
	if r.reads != reads {
		t.Fatal("EOF accompanying the final sentinel was not retained")
	}

	checkStreamFuzzOwnedResults(t, d, r, payload, ownedText, ownedBytes)
}

func checkStreamFuzzOwnedResults(t *testing.T, d *Reader, r *streamFuzzReader, payload []byte, ownedText string, ownedBytes []byte) {
	t.Helper()
	// Mutate all source bytes, refill the decoder's copy buffer after Reset,
	// and decode another owned result before checking the retained results.
	for index := range r.data {
		r.data[index] ^= 0xff
	}
	d.Reset(bytes.NewReader([]byte{0xc4, 3, 1, 2, 3, 0xc4, 1, 9}))
	if n, err := d.DecodeBytesTo(io.Discard); n != 3 || err != nil {
		t.Fatalf("reset copy-buffer reuse = (%d, %v)", n, err)
	}

	later, err := d.DecodeBytes()
	if err != nil || len(later) != 1 {
		t.Fatalf("later owned result = (%x, %v)", later, err)
	}

	later[0] = 7
	if ownedText != string(payload) || !bytes.Equal(ownedBytes, payload) || len(d.state.buffer) > 32768 {
		t.Fatal("owned results changed after later values, source mutation, scratch reuse, or Reset")
	}
}
