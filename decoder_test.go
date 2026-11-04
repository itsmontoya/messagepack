package messagepack

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"testing"
)

func TestDecoderDecodee(t *testing.T) {
	var (
		fixture = applicationMessageFixture()
		input   = &decoderTestReader{data: bytes.Repeat(fixture, 2), maxRead: 1}
		d       = NewDecoder(input)
		message applicationMessage
		want    = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
			Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
		err error
	)
	for count := 1; count <= 2; count++ {
		if err = d.Decode(&message); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(message, want) || input.position != count*len(fixture) {
			t.Fatalf("Decode call %d = %+v, position %d", count, message, input.position)
		}
	}
	if input.closes != 0 {
		t.Fatal("Decode closed its input")
	}
}

func TestDecoderUnsupportedValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "nil"},
		{name: "bool", value: new(bool)},
		{name: "integer", value: new(int64)},
		{name: "string", value: new(string)},
		{name: "bytes", value: new([]byte)},
		{name: "struct", value: &struct{ ID int }{}},
		{name: "map", value: map[string]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				input = &decoderTestReader{data: []byte{0xc3}}
				d     = NewDecoder(input)
				value = &decoderTestDecodee{decode: func(r Reader) (err error) {
					_, err = r.DecodeBool()
					return err
				}}
				err = d.Decode(tt.value)
			)
			if err != ErrUnsupportedType || input.reads != 0 {
				t.Fatalf("Decode(%T) = %v, reads %d", tt.value, err, input.reads)
			}
			if err = d.Decode(value); err != nil || value.calls != 1 || input.position != 1 {
				t.Fatalf("Decode after unsupported target = %v, calls %d, position %d", err, value.calls, input.position)
			}
		})
	}
}

func TestDecoderTypedNilDecodee(t *testing.T) {
	var value *decoderTestDecodee
	if err := NewDecoder(bytes.NewReader([]byte{0xc0})).Decode(value); err != nil {
		t.Fatal(err)
	}
}

func TestDecoderFailuresAreTerminal(t *testing.T) {
	var (
		customFailure = errors.New("test: custom decoding failure")
		sourceFailure = errors.New("test: source failure")
		tests         = []struct {
			name    string
			wire    []byte
			endErr  error
			decode  func(r Reader) (err error)
			wantErr error
		}{
			{name: "custom before input", wire: []byte{0x91}, wantErr: customFailure,
				decode: func(r Reader) (err error) { return customFailure }},
			{name: "custom after input", wire: []byte{0x91}, wantErr: customFailure,
				decode: func(r Reader) (err error) {
					if _, err = r.DecodeArrayHeader(); err != nil {
						return err
					}
					return customFailure
				}},
			{name: "source", endErr: sourceFailure, wantErr: sourceFailure,
				decode: func(r Reader) (err error) { return r.DecodeNil() }},
			{name: "empty input", wantErr: io.EOF,
				decode: func(r Reader) (err error) { return r.DecodeNil() }},
			{name: "truncated integer", wire: []byte{0xcd, 1}, wantErr: io.ErrUnexpectedEOF,
				decode: func(r Reader) (err error) { _, err = r.DecodeUint64(); return err }},
			{name: "classification", wire: []byte{0xc1}, wantErr: ErrInvalidFormat,
				decode: func(r Reader) (err error) { return r.DecodeNil() }},
			{name: "ignored read failure", endErr: sourceFailure, wantErr: sourceFailure,
				decode: func(r Reader) (err error) { _ = r.DecodeNil(); return nil }},
			{name: "schema error after read failure", endErr: sourceFailure, wantErr: customFailure,
				decode: func(r Reader) (err error) { _ = r.DecodeNil(); return customFailure }},
		}
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				input = &decoderTestReader{data: tt.wire, endErr: tt.endErr}
				d     = NewDecoder(input)
				value = &decoderTestDecodee{decode: tt.decode}
				first = d.Decode(value)
				reads = input.reads
				err   error
			)
			if !errors.Is(first, tt.wantErr) {
				t.Fatalf("Decode = %v, want %v", first, tt.wantErr)
			}
			if tt.wantErr == ErrInvalidFormat && first != tt.wantErr {
				t.Fatal("Reader classification sentinel was wrapped")
			}
			if err = d.Decode(value); err != first || value.calls != 1 || input.reads != reads {
				t.Fatalf("terminal Decode = %v, calls %d, reads %d", err, value.calls, input.reads)
			}
			for count := 0; count < 2; count++ {
				if err = d.Close(); err != first || input.closes != 0 || input.reads != reads {
					t.Fatalf("Close = %v, closes %d, reads %d", err, input.closes, input.reads)
				}
			}
		})
	}
}

func TestDecoderCloseLeavesInputOpen(t *testing.T) {
	var (
		input = &decoderTestReader{data: []byte{0xc0, 0xc3}}
		d     = NewDecoder(input)
		value *decoderTestDecodee
		err   = d.Decode(value)
		reads = input.reads
		tail  [1]byte
		n     int
	)
	if err != nil {
		t.Fatal(err)
	}
	for count := 0; count < 2; count++ {
		if err = d.Close(); err != nil || input.closes != 0 || input.reads != reads {
			t.Fatalf("Close = %v, closes %d, reads %d", err, input.closes, input.reads)
		}
	}
	if err = d.Decode(value); err != ErrClosed || input.reads != reads {
		t.Fatalf("Decode after Close = %v, reads %d", err, input.reads)
	}
	n, err = input.Read(tail[:])
	if err != nil || n != 1 || tail[0] != 0xc3 {
		t.Fatalf("input after Close = (%x, %d, %v)", tail, n, err)
	}
}

func TestDecoderUninitialized(t *testing.T) {
	for _, d := range []*Decoder{{}, NewDecoder(nil)} {
		var value *decoderTestDecodee
		if err := d.Decode(value); err != ErrUninitialized {
			t.Fatalf("unattached Decode = %v", err)
		}
		if err := d.Close(); err != nil {
			t.Fatal(err)
		}
		if err := d.Decode(value); err != ErrClosed {
			t.Fatalf("closed Decode = %v", err)
		}
	}
}

func TestDecoderPublicMethods(t *testing.T) {
	var (
		typ  = reflect.TypeOf(&Decoder{})
		want = []string{"Close", "Decode"}
	)
	if typ.NumMethod() != len(want) {
		t.Fatalf("Decoder exposes %d methods, want %v", typ.NumMethod(), want)
	}
	for index, name := range want {
		if typ.Method(index).Name != name {
			t.Fatalf("Decoder method %d = %s, want %s", index, typ.Method(index).Name, name)
		}
	}
}
