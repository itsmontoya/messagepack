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

func TestStreamContainerHeaderOnlyAndWiderForms(t *testing.T) {
	tests := []struct {
		name  string
		wire  []byte
		isMap bool
		count uint32
	}{
		{name: "array16 empty", wire: []byte{0xdc, 0, 0}},
		{name: "array32 one", wire: []byte{0xdd, 0, 0, 0, 1}, count: 1},
		{name: "map16 empty", wire: []byte{0xde, 0, 0}, isMap: true},
		{name: "map32 one", wire: []byte{0xdf, 0, 0, 0, 1}, isMap: true, count: 1},
		{name: "array max no children", wire: []byte{0xdd, 0xff, 0xff, 0xff, 0xff}, count: math.MaxUint32},
		{name: "map max no children", wire: []byte{0xdf, 0xff, 0xff, 0xff, 0xff}, isMap: true, count: math.MaxUint32},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				r     = &decoderTestReader{data: tt.wire, endErr: io.EOF}
				d     = NewReader(r)
				count uint32
				err   error
			)
			if tt.isMap {
				count, err = d.DecodeMapHeader()
			} else {
				count, err = d.DecodeArrayHeader()
			}
			if err != nil || count != tt.count || r.position != len(tt.wire) || d.state.buffer != nil {
				t.Fatalf("header-only read = (%d, %v), consumed %d, buffer %d", count, err, r.position, len(d.state.buffer))
			}
			reads := r.reads
			if err = d.DecodeNil(); !errors.Is(err, io.EOF) || r.reads != reads {
				t.Fatalf("pending EOF = %v, reads %d, want %d", err, r.reads, reads)
			}
		})
	}
}

func TestStreamContainerTruncationAndClassification(t *testing.T) {
	for _, isMap := range []bool{false, true} {
		wires := [][]byte{{0x90}, {0xdc, 0, 16}, {0xdd, 0, 1, 0, 0}}
		if isMap {
			wires = [][]byte{{0x80}, {0xde, 0, 16}, {0xdf, 0, 1, 0, 0}}
		}
		for _, wire := range wires {
			for length := 0; length < len(wire); length++ {
				t.Run(fmt.Sprintf("map_%t/tag_%x/length_%d", isMap, wire[0], length), func(t *testing.T) {
					var (
						r     = &decoderTestReader{data: wire[:length], maxRead: 1}
						d     = NewReader(r)
						count uint32
						err   error
						want  = io.ErrUnexpectedEOF
					)
					if length == 0 {
						want = io.EOF
					}
					if isMap {
						count, err = d.DecodeMapHeader()
					} else {
						count, err = d.DecodeArrayHeader()
					}
					if count != 0 || !errors.Is(err, want) || errors.Is(err, ErrTruncated) || r.position != length {
						t.Fatalf("truncated header = (%d, %v), position %d", count, err, r.position)
					}
					reads := r.reads
					requireStreamHeaderFailure(t, d, err)
					if r.reads != reads {
						t.Fatal("terminal header failure performed further reads")
					}
				})
			}
		}
		for _, tag := range []byte{0xc1, 0xc7, 0xc8, 0xc9, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xc0, 0xdb, 0x90, 0x80, 0xdd, 0xdf} {
			if isMap && (tag == 0x80 || tag == 0xdf) || !isMap && (tag == 0x90 || tag == 0xdd) {
				continue
			}
			var (
				r     = &decoderTestReader{data: []byte{tag, 0xc3}}
				d     = NewReader(r)
				count uint32
				err   error
				want  = readTypeError(tag)
			)
			if isMap {
				count, err = d.DecodeMapHeader()
			} else {
				count, err = d.DecodeArrayHeader()
			}
			if count != 0 || err != want || r.position != 1 {
				t.Fatalf("map %t rejected tag %x = (%d, %v), consumed %d, want %v", isMap, tag, count, err, r.position, want)
			}
		}
	}
}

func requireStreamHeaderFailure(t *testing.T, d *Reader, want error) {
	t.Helper()
	array, arrayErr := d.DecodeArrayHeader()
	pairs, pairsErr := d.DecodeMapHeader()
	if array != 0 || pairs != 0 || arrayErr != want || pairsErr != want {
		t.Fatalf("stored header error = (%d, %v), (%d, %v), want %v", array, arrayErr, pairs, pairsErr, want)
	}
	requireReaderScalarFailure(t, d, want)
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
