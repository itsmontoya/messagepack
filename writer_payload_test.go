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

func TestWriterPayloadWire(t *testing.T) {
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
			n, err := method.run(e, 1, nil)
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
