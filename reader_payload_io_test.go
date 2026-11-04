package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestReaderPayloadClassification(t *testing.T) {
	methods := []struct {
		name  string
		run   func(d *Reader) (err error)
		wrong byte
	}{
		{name: "string", wrong: 0xc6, run: func(d *Reader) (err error) {
			value, readErr := d.DecodeString()
			if value != "" {
				return fmt.Errorf("failed string result %q is not zero", value)
			}

			return readErr
		}},
		{name: "binary", wrong: 0xdb, run: func(d *Reader) (err error) {
			value, readErr := d.DecodeBytes()
			if value != nil {
				return fmt.Errorf("failed binary result %x is not nil", value)
			}

			return readErr
		}},
		{name: "string to", wrong: 0xc6, run: func(d *Reader) (err error) {
			n, readErr := d.DecodeStringTo(io.Discard)
			if n != 0 {
				return fmt.Errorf("failed string progress %d is not zero", n)
			}

			return readErr
		}},
		{name: "binary to", wrong: 0xdb, run: func(d *Reader) (err error) {
			n, readErr := d.DecodeBytesTo(io.Discard)
			if n != 0 {
				return fmt.Errorf("failed binary progress %d is not zero", n)
			}

			return readErr
		}},
	}
	for _, method := range methods {
		for _, tag := range []byte{0xc1, 0xc7, 0xc8, 0xc9, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8, 0xc0, 0xc3, 0x01, 0xff, 0xcf, 0xcb, 0x90, 0xdd, 0x80, 0xdf, method.wrong} {
			t.Run(fmt.Sprintf("%s/tag_%x", method.name, tag), func(t *testing.T) {
				var (
					r    = &decoderTestReader{data: []byte{tag, 0xc3}}
					d    = NewReader(r)
					want = readTypeError(tag)
					err  = method.run(d)
				)
				if err != want || r.position != 1 {
					t.Fatalf("classification = %v, consumed %d, want %v and one byte", err, r.position, want)
				}

				reads := r.reads
				requireReaderPayloadFailure(t, d, err)
				if r.reads != reads {
					t.Fatal("classification failure performed more I/O")
				}
			})
		}
	}
}

func TestReaderPayloadTransferValidationAndEmpty(t *testing.T) {
	methods := []struct {
		name  string
		empty []byte
		wire  []byte
		run   func(d *Reader, dst io.Writer) (n uint64, err error)
	}{
		{name: "string", empty: []byte{0xa0}, wire: []byte{0xa2, 1, 2}, run: (*Reader).DecodeStringTo},
		{name: "binary", empty: []byte{0xc4, 0}, wire: []byte{0xc4, 2, 1, 2}, run: (*Reader).DecodeBytesTo},
	}
	for _, method := range methods {
		t.Run(method.name, func(t *testing.T) {
			var (
				unattached Reader
				r          = &decoderTestReader{data: append(append([]byte(nil), method.empty...), 0xc3)}
				d          = NewReader(r)
				writer     = &encoderPayloadWriter{failCall: 1, err: io.ErrClosedPipe}
			)
			if n, err := method.run(&unattached, nil); n != 0 || err != ErrUninitialized {
				t.Fatalf("unattached nil transfer = (%d, %v)", n, err)
			}

			if n, err := method.run(d, nil); n != 0 || err != ErrInvalidArgument || r.reads != 0 || d.state.err != nil {
				t.Fatalf("nil destination = (%d, %v), reads %d", n, err, r.reads)
			}

			if n, err := method.run(d, writer); n != 0 || err != nil || writer.writes != 0 || d.state.buffer != nil || r.position != len(method.empty) {
				t.Fatalf("empty transfer = (%d, %v), writes %d, consumed %d", n, err, writer.writes, r.position)
			}

			if next, err := d.DecodeBool(); !next || err != nil {
				t.Fatalf("following value after empty = (%v, %v)", next, err)
			}

			d.Reset(bytes.NewReader(method.wire))
			if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: 1}); err != nil {
				t.Fatal(err)
			}

			var typedNil *encoderTestWriter
			if n, err := method.run(d, typedNil); n != 2 || err != nil {
				t.Fatalf("valid typed-nil writer/independent cap = (%d, %v)", n, err)
			}
		})
	}
}

func TestReaderPayloadSourceErrors(t *testing.T) {
	var (
		failure = errors.New("test payload source failure")
		tests   = []struct {
			name  string
			tail  []byte
			end   error
			want  error
			wantN uint64
		}{
			{name: "no payload error", end: failure, want: failure},
			{name: "partial payload error", tail: []byte{1, 2}, end: failure, want: failure, wantN: 2},
			{name: "full payload error", tail: []byte{1, 2, 3, 4}, end: failure, want: failure, wantN: 4},
			{name: "no payload EOF", end: io.EOF, want: io.ErrUnexpectedEOF},
			{name: "partial payload EOF", tail: []byte{1, 2}, end: io.EOF, want: io.ErrUnexpectedEOF, wantN: 2},
			{name: "full payload EOF", tail: []byte{1, 2, 3, 4}, end: io.EOF, wantN: 4},
		}
	)
	for _, isString := range []bool{false, true} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("string_%t/%s", isString, tt.name), func(t *testing.T) {
				var header []byte
				if isString {
					header = []byte{0xd9, 4}
				} else {
					header = []byte{0xc4, 4}
				}

				var (
					wire = append(append([]byte(nil), header...), tt.tail...)
					r    = &decoderTestReader{data: wire, endErr: tt.end}
					d    = NewReader(r)
					err  error
				)
				if isString {
					value, readErr := d.DecodeString()
					err = readErr
					if tt.want != nil && value != "" {
						t.Fatalf("failed owned string = %q", value)
					}
				} else {
					value, readErr := d.DecodeBytes()
					err = readErr
					if tt.want != nil && value != nil {
						t.Fatalf("failed owned binary = %x", value)
					}
				}

				if !errors.Is(err, tt.want) || errors.Is(err, errFuzzTruncated) {
					t.Fatalf("owned source error = %v, want %v", err, tt.want)
				}

				r = &decoderTestReader{data: wire, endErr: tt.end}
				d.Reset(r)
				var (
					dst bytes.Buffer
					n   uint64
				)
				if isString {
					n, err = d.DecodeStringTo(&dst)
				} else {
					n, err = d.DecodeBytesTo(&dst)
				}

				if !errors.Is(err, tt.want) || n != tt.wantN || !bytes.Equal(dst.Bytes(), tt.tail) {
					t.Fatalf("source transfer = (%d, %v), payload %x", n, err, dst.Bytes())
				}

				if tt.want != nil {
					if err == tt.want || !strings.Contains(err.Error(), "source") || !strings.Contains(err.Error(), "decode") {
						t.Fatalf("source error lacks context: %v", err)
					}

					reads := r.reads
					requireReaderPayloadFailure(t, d, err)
					if reads != r.reads {
						t.Fatal("source failure performed more I/O")
					}
				}
			})
		}
	}
}

func TestReaderPayloadHeaderErrors(t *testing.T) {
	failure := errors.New("test payload header failure")
	for _, wire := range [][]byte{{0xa0}, {0xd9, 0}, {0xda, 0, 0}, {0xdb, 0, 0, 0, 0}, {0xc4, 0}, {0xc5, 0, 0}, {0xc6, 0, 0, 0, 0}} {
		r := &decoderTestReader{data: wire, endErr: failure}
		d := NewReader(r)
		var (
			n   uint64
			err error
		)
		if wire[0] >= 0xa0 && wire[0] <= 0xbf || wire[0] >= 0xd9 && wire[0] <= 0xdb {
			n, err = d.DecodeStringTo(io.Discard)
		} else {
			n, err = d.DecodeBytesTo(io.Discard)
		}

		if n != 0 || !errors.Is(err, failure) {
			t.Fatalf("full header plus error for %x = (%d, %v)", wire, n, err)
		}
	}
}

func TestReaderPayloadDestinationFailures(t *testing.T) {
	var (
		sourceFailure      = errors.New("test simultaneous source failure")
		destinationFailure = errors.New("test payload destination failure")
		tests              = []struct {
			name      string
			accept    int
			acceptAll bool
			writeErr  error
			readErr   error
			want      error
			wantN     uint64
		}{
			{name: "before output", writeErr: destinationFailure, want: destinationFailure},
			{name: "partial error", accept: 2, writeErr: destinationFailure, want: destinationFailure, wantN: 2},
			{name: "full error", acceptAll: true, writeErr: destinationFailure, want: destinationFailure, wantN: 4},
			{name: "short nil", accept: 2, want: io.ErrShortWrite, wantN: 2},
			{name: "zero progress", want: io.ErrShortWrite},
			{name: "simultaneous partial", accept: 2, writeErr: destinationFailure, readErr: sourceFailure, want: destinationFailure, wantN: 2},
			{name: "simultaneous full", acceptAll: true, writeErr: destinationFailure, readErr: sourceFailure, want: destinationFailure, wantN: 4},
			{name: "simultaneous short nil", accept: 2, readErr: sourceFailure, want: io.ErrShortWrite, wantN: 2},
		}
	)
	for _, method := range []struct {
		name   string
		header []byte
		run    func(d *Reader, dst io.Writer) (n uint64, err error)
	}{
		{name: "string", header: []byte{0xa4}, run: (*Reader).DecodeStringTo},
		{name: "binary", header: []byte{0xc4, 4}, run: (*Reader).DecodeBytesTo},
	} {
		for _, tt := range tests {
			t.Run(method.name+"/"+tt.name, func(t *testing.T) {
				var (
					r      = &decoderTestReader{data: append(append([]byte(nil), method.header...), 1, 2, 3, 4), endErr: tt.readErr}
					d      = NewReader(r)
					writer = &encoderPayloadWriter{failCall: 1, accept: tt.accept, acceptAll: tt.acceptAll, err: tt.writeErr}
					n, err = method.run(d, writer)
				)
				if n != tt.wantN || !errors.Is(err, tt.want) || errors.Is(err, sourceFailure) || !bytes.Equal(writer.data, []byte{1, 2, 3, 4}[:int(tt.wantN)]) {
					t.Fatalf("destination failure = (%d, %v), accepted %x", n, err, writer.data)
				}

				if err == tt.want || !strings.Contains(err.Error(), "destination") || !strings.Contains(err.Error(), "decode") {
					t.Fatalf("destination error lacks context: %v", err)
				}

				reads := r.reads
				requireReaderPayloadFailure(t, d, err)
				if r.reads != reads || writer.writes != 1 || writer.flushes != 0 || writer.closes != 0 {
					t.Fatal("destination failure retried/drained input or changed writer ownership")
				}
			})
		}
	}
}

func TestReaderPayloadZeroProgress(t *testing.T) {
	for _, empty := range []int{99, 100} {
		for _, isString := range []bool{false, true} {
			var header []byte
			if isString {
				header = []byte{0xa2}
			} else {
				header = []byte{0xc4, 2}
			}

			var (
				source = &decoderTestReader{data: []byte{1, 2}, maxRead: 1, initialEmpty: empty, emptyBetween: empty}
				d      = NewReader(io.MultiReader(bytes.NewReader(header), source))
				dst    bytes.Buffer
				n      uint64
				err    error
			)
			if isString {
				n, err = d.DecodeStringTo(&dst)
			} else {
				n, err = d.DecodeBytesTo(&dst)
			}

			if empty == 99 {
				if n != 2 || err != nil || !bytes.Equal(dst.Bytes(), []byte{1, 2}) {
					t.Fatalf("temporary empty reads = (%d, %v), bytes %x", n, err, dst.Bytes())
				}

				continue
			}

			if n != 0 || !errors.Is(err, io.ErrNoProgress) || source.reads != 100 || dst.Len() != 0 {
				t.Fatalf("zero progress = (%d, %v), reads %d", n, err, source.reads)
			}

			requireReaderPayloadFailure(t, d, err)
		}
	}
}

func TestReaderGeneratedPayloadTransfer(t *testing.T) {
	for _, method := range []struct {
		name   string
		header []byte
		run    func(d *Reader, dst io.Writer) (n uint64, err error)
	}{
		{name: "string", header: []byte{0xdb, 0, 0x80, 0, 7}, run: (*Reader).DecodeStringTo},
		{name: "binary", header: []byte{0xc6, 0, 0x80, 0, 7}, run: (*Reader).DecodeBytesTo},
	} {
		t.Run(method.name, func(t *testing.T) {
			const length = 8*1024*1024 + 7
			var (
				source = &encoderGeneratedReader{length: length}
				reader = io.MultiReader(bytes.NewReader(method.header), source, bytes.NewReader([]byte{0xc3}))
				d      = NewReader(reader)
				writer = &decoderCountingWriter{source: source}
			)
			if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: 1}); err != nil {
				t.Fatal(err)
			}

			n, err := method.run(d, writer)
			if err != nil || n != length || source.read != length || writer.bytes != length || writer.invalid {
				t.Fatalf("generated transfer = (%d, %v), source %d, sink %d, invalid %v", n, err, source.read, writer.bytes, writer.invalid)
			}

			if source.maxRequest > 32768 || writer.maxRequest > 32768 || writer.firstPayloadSourceRead > 32768 || writer.firstPayloadSourceRead >= length || len(d.state.buffer) > 32768 {
				t.Fatalf("transfer was not bounded/incremental: source request %d, sink request %d, first output after %d", source.maxRequest, writer.maxRequest, writer.firstPayloadSourceRead)
			}

			if next, nextErr := d.DecodeBool(); !next || nextErr != nil {
				t.Fatalf("following generated-payload value = (%v, %v)", next, nextErr)
			}
		})
	}
}

func TestReaderPayloadFailureDoesNotDrain(t *testing.T) {
	for _, failCall := range []int{1, 2} {
		var (
			source   = &encoderGeneratedReader{length: 8 * 1024 * 1024}
			d        = NewReader(io.MultiReader(bytes.NewReader([]byte{0xc6, 0, 0x80, 0, 0}), source))
			writer   = &encoderPayloadWriter{failCall: failCall, accept: 3, err: io.ErrClosedPipe}
			n, err   = d.DecodeBytesTo(writer)
			wantN    = uint64(failCall-1)*32768 + 3
			wantRead = uint64(failCall) * 32768
		)
		if n != wantN || !errors.Is(err, io.ErrClosedPipe) || source.read != wantRead || source.read == source.length {
			t.Fatalf("failed chunk %d = (%d, %v), source read %d", failCall, n, err, source.read)
		}

		reads := source.reads
		requireReaderPayloadFailure(t, d, err)
		if source.reads != reads || writer.writes != failCall {
			t.Fatal("failed generated payload retried or drained")
		}
	}
}

func TestReaderPayloadLaterChunkSourceFailure(t *testing.T) {
	failure := errors.New("test later payload source failure")
	for _, tag := range []byte{0xdb, 0xc6} {
		var (
			wire = append([]byte{tag, 0, 1, 0, 0}, bytes.Repeat([]byte{0x78}, 32771)...)
			r    = &decoderTestReader{data: wire, endErr: failure}
			d    = NewReader(r)
			dst  bytes.Buffer
			n    uint64
			err  error
		)
		if tag == 0xdb {
			n, err = d.DecodeStringTo(&dst)
		} else {
			n, err = d.DecodeBytesTo(&dst)
		}

		if n != 32771 || !errors.Is(err, failure) || dst.Len() != 32771 || r.position != len(wire) {
			t.Fatalf("later source transfer = (%d, %v), output %d, consumed %d", n, err, dst.Len(), r.position)
		}

		d.Reset(&decoderTestReader{data: wire, endErr: failure})
		if tag == 0xdb {
			value, readErr := d.DecodeString()
			if value != "" || !errors.Is(readErr, failure) {
				t.Fatalf("later owned string failure = (%q, %v)", value, readErr)
			}

			continue
		}

		value, readErr := d.DecodeBytes()
		if value != nil || !errors.Is(readErr, failure) {
			t.Fatalf("later owned binary failure = (%x, %v)", value, readErr)
		}
	}
}

func TestReaderPayloadNoProgressRetainsPartialCount(t *testing.T) {
	var (
		source = &decoderTestReader{data: []byte{1, 2}, maxRead: 1, emptyBetween: 100}
		d      = NewReader(io.MultiReader(bytes.NewReader([]byte{0xc4, 2}), source))
		dst    bytes.Buffer
		n, err = d.DecodeBytesTo(&dst)
	)
	if n != 1 || !errors.Is(err, io.ErrNoProgress) || !bytes.Equal(dst.Bytes(), []byte{1}) || source.reads != 101 {
		t.Fatalf("partial no progress = (%d, %v), output %x, reads %d", n, err, dst.Bytes(), source.reads)
	}
}

func TestReaderHugeTransferHeaderDoesNotMaterialize(t *testing.T) {
	for _, tag := range []byte{0xdb, 0xc6} {
		r := &decoderTestReader{data: []byte{tag, 0xff, 0xff, 0xff, 0xff, 1, 2}}
		d := NewReader(r)
		var (
			n   uint64
			err error
			dst bytes.Buffer
		)
		if tag == 0xdb {
			n, err = d.DecodeStringTo(&dst)
		} else {
			n, err = d.DecodeBytesTo(&dst)
		}

		if n != 2 || !errors.Is(err, io.ErrUnexpectedEOF) || !bytes.Equal(dst.Bytes(), []byte{1, 2}) || len(d.state.buffer) != 32768 {
			t.Fatalf("huge transfer header = (%d, %v), payload %x, scratch %d", n, err, dst.Bytes(), len(d.state.buffer))
		}
	}
}
