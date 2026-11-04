package messagepack

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func FuzzStreamPayloadTransfer(f *testing.F) {
	for _, wire := range streamFuzzSeeds() {
		for family := byte(0); family < 2; family++ {
			f.Add(wire, []byte{family})
			for length := 0; length < len(wire); length++ {
				f.Add(wire[:length], []byte{family, 0, 0, 0, 1, 1})
			}
			for _, empty := range []byte{0, 1, 99, 100, 101} {
				for _, mode := range []byte{1, 2} {
					f.Add(wire, []byte{family, 63, empty, 0, mode, 1, 2, 3, 0})
				}
			}
		}
	}
	f.Fuzz(func(t *testing.T, src, controls []byte) {
		if len(src) > streamFuzzInputLimit || len(controls) > 32 {
			t.Skip()
		}
		var settings [12]byte
		copy(settings[:], controls)
		checkStreamFuzzDecodeTransfer(t, src, controls, settings)
		checkStreamFuzzEncodeTransfer(t, src, controls, settings)
	})
}

func checkStreamFuzzDecodeTransfer(t *testing.T, src, fragments []byte, settings [12]byte) {
	t.Helper()
	var (
		before   = append([]byte(nil), src...)
		isString = settings[0]&1 != 0
		r        = &streamFuzzReader{data: src, fragments: fragments, empty: int(settings[2]) % 102,
			emptyBetween: int(settings[3]) % 102, dataError: settings[5]&1 != 0}
		w = &streamFuzzWriter{fail: settings[6]%3 != 0,
			failAt: int(binary.LittleEndian.Uint16(settings[7:9])) % (len(src) + 6)}
		d        = NewReader(r)
		expected []byte
		consumed int
		want     error
		n        uint64
		err      error
	)
	if settings[4]%3 == 1 {
		r.endErr = io.EOF
	}
	if settings[4]%3 == 2 {
		r.endErr = errStreamFuzzSource
	}
	if settings[6]%3 == 2 {
		w.err = errStreamFuzzDestination
	}
	if err = d.SetLimits(ReaderLimits{MaxPayloadBytes: 1}); err != nil {
		t.Fatal(err)
	}
	length, header, headerErr := streamFuzzPayloadHeader(src, isString)
	want = headerErr
	if headerErr == nil {
		if length > uint64(len(src)-header) {
			want = errFuzzTruncated
		} else {
			consumed = header + int(length)
			expected = src[header:consumed]
		}
	}
	if isString {
		n, err = d.DecodeStringTo(w)
	} else {
		n, err = d.DecodeBytesTo(w)
	}
	want = streamFuzzError(want, len(src))
	if r.noProgress {
		want = io.ErrNoProgress
	}
	if r.deliveredError == errStreamFuzzSource {
		want = errStreamFuzzSource
	}
	if w.hit {
		want = w.err
		if want == nil {
			want = io.ErrShortWrite
		}
	}
	requireStreamFuzzError(t, err, want)
	requireStreamFuzzInput(t, src, before, r, d)
	if n != uint64(len(w.data)) || n > uint64(len(src)) {
		t.Fatalf("transfer accepted count = %d, stored %d bytes", n, len(w.data))
	}
	if n > 0 && (headerErr != nil || n > length || header+int(n) > r.position || !bytes.Equal(w.data, src[header:header+int(n)])) {
		t.Fatal("transfer output is not the accepted original payload prefix")
	}
	if err == nil {
		if r.position != consumed || !bytes.Equal(w.data, expected) || n != uint64(len(expected)) {
			t.Fatalf("successful transfer consumed %d, slice %d, accepted %d", r.position, consumed, n)
		}
		return
	}
	reads, writes := r.reads, w.writes
	if later, laterErr := d.DecodeBytesTo(w); later != 0 || laterErr != err || r.reads != reads || w.writes != writes {
		t.Fatal("failed transfer resumed source or destination I/O")
	}
	requireStreamFuzzTerminal(t, d, r, err)
}

func checkStreamFuzzEncodeTransfer(t *testing.T, src, fragments []byte, settings [12]byte) {
	t.Helper()
	var (
		before   = append([]byte(nil), src...)
		isString = settings[0]&1 != 0
		length   = uint64(len(src))
		r        = &streamFuzzReader{data: src, fragments: fragments, empty: int(settings[2]) % 102,
			emptyBetween: int(settings[3]) % 102, dataError: settings[5]&1 != 0}
		w = &streamFuzzWriter{fail: settings[6]%3 != 0,
			failAt: int(binary.LittleEndian.Uint16(settings[7:9])) % (len(src) + 6)}
		e      = NewWriter(w)
		header []byte
		n      uint64
		err    error
		want   error
	)
	switch settings[9] % 6 {
	case 1:
		length++
	case 2:
		length = uint64(binary.LittleEndian.Uint16(settings[10:12]))
	case 3:
		length = 4294967295
	case 4:
		length = 4294967296
	case 5:
		length = 0
	}
	if settings[4]%3 == 1 {
		r.endErr = io.EOF
	}
	if settings[4]%3 == 2 {
		r.endErr = errStreamFuzzSource
	}
	if settings[6]%3 == 2 {
		w.err = errStreamFuzzDestination
	}
	if isString {
		n, err = e.EncodeStringFrom(length, r)
	} else {
		n, err = e.EncodeBytesFrom(length, r)
	}
	if length > 4294967295 {
		if err != ErrOverflow || n != 0 || r.reads != 0 || w.writes != 0 || e.state.err != nil {
			t.Fatal("oversized encode length performed I/O or became terminal")
		}
		e.Reset(io.Discard)
		if err = e.EncodeNil(); err != nil {
			t.Fatal(err)
		}
		return
	}
	// Construct the expected header independently of Writer's scratch encoding.
	tags := [3]byte{0xc4, 0xc5, 0xc6}
	if isString {
		tags = [3]byte{0xd9, 0xda, 0xdb}
	}
	switch {
	case isString && length <= 31:
		header = []byte{0xa0 | byte(length)}
	case length <= 255:
		header = []byte{tags[0], byte(length)}
	case length <= 65535:
		header = binary.BigEndian.AppendUint16([]byte{tags[1]}, uint16(length))
	default:
		header = binary.BigEndian.AppendUint32([]byte{tags[2]}, uint32(length))
	}
	if length > uint64(len(src)) {
		want = io.ErrUnexpectedEOF
	}
	if r.noProgress {
		want = io.ErrNoProgress
	}
	if r.deliveredError == errStreamFuzzSource {
		want = errStreamFuzzSource
	}
	if w.hit {
		want = w.err
		if want == nil {
			want = io.ErrShortWrite
		}
	}
	requireStreamFuzzError(t, err, want)
	var (
		available = append(append([]byte(nil), header...), src[:r.position]...)
		accepted  = len(w.data) - len(header)
	)
	if accepted < 0 {
		accepted = 0
	}
	if !bytes.Equal(src, before) || len(w.data) > len(available) || !bytes.Equal(w.data, available[:len(w.data)]) || n != uint64(accepted) {
		t.Fatal("encode transfer changed input, wire prefix, or payload-only progress")
	}
	if len(e.state.buffer) > 32768 || r.position > len(src) || r.reads > (len(src)+1)*101+102 {
		t.Fatal("encode transfer exceeded bounded memory or scheduled work")
	}
	if err == nil {
		if n != length || uint64(r.position) != length || len(w.data) != len(header)+int(length) {
			t.Fatal("successful encode read past its declared payload boundary")
		}
		return
	}
	requireStreamFuzzWriterTerminal(t, e, r, w, err)
}

func requireStreamFuzzWriterTerminal(t *testing.T, e *Writer, r *streamFuzzReader, w *streamFuzzWriter, failure error) {
	t.Helper()
	reads, writes := r.reads, w.writes
	if e.EncodeNil() != failure || e.EncodeBool(true) != failure || e.EncodeInt64(-1) != failure || e.EncodeUint64(1) != failure ||
		e.EncodeFloat32(1) != failure || e.EncodeFloat64(1) != failure || e.EncodeString("") != failure || e.EncodeBytes(nil) != failure ||
		e.EncodeArrayHeader(0) != failure || e.EncodeMapHeader(0) != failure {
		t.Fatal("encoder lost its stored failure")
	}
	if n, err := e.EncodeBytesFrom(1, r); n != 0 || err != failure {
		t.Fatal("encoder resumed a real source after failure")
	}
	if n, err := e.EncodeStringFrom(4294967296, nil); n != 0 || err != failure || r.reads != reads || w.writes != writes {
		t.Fatal("encoder terminal validation lost precedence or performed I/O")
	}
	e.Reset(nil)
	if e.EncodeNil() != ErrUninitialized || w.writes != writes {
		t.Fatal("nil encoder Reset retained attachment or wrote output")
	}
	var fresh streamFuzzWriter
	e.Reset(&fresh)
	if fresh.writes != 0 || e.EncodeNil() != nil || !bytes.Equal(fresh.data, []byte{0xc0}) || r.reads != reads || w.writes != writes {
		t.Fatal("encoder Reset did not isolate old I/O and clear failure")
	}
}
