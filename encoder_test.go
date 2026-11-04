package messagepack

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestEncoderEncodee(t *testing.T) {
	var (
		output = &encoderTestWriter{}
		e      = NewEncoder(output)
		value  = &encoderTestEncodee{encode: func(w Writer) (err error) {
			if err = w.EncodeArrayHeader(3); err != nil {
				return err
			}

			if err = w.EncodeString("id"); err != nil {
				return err
			}

			if err = w.EncodeUint64(42); err != nil {
				return err
			}

			return w.EncodeBytes([]byte{1, 2})
		}}
		want = []byte{0x93, 0xa2, 'i', 'd', 0x2a, 0xc4, 0x02, 0x01, 0x02}
		err  error
	)

	for count := 1; count <= 2; count++ {
		if err = e.Encode(value); err != nil {
			t.Fatal(err)
		}

		if value.calls != count || !bytes.Equal(output.data, bytes.Repeat(want, count)) {
			t.Fatalf("Encode call %d = (%x, %d dispatches)", count, output.data, value.calls)
		}
	}
	if output.flushes != 0 || output.closes != 0 {
		t.Fatal("Encode flushed or closed its output")
	}
}

func TestEncoderTypedNilEncodee(t *testing.T) {
	var (
		output bytes.Buffer
		value  *encoderTestEncodee
		err    = NewEncoder(&output).Encode(value)
	)

	if err != nil || !bytes.Equal(output.Bytes(), []byte{0xc0}) {
		t.Fatalf("Encode(typed nil) = (%x, %v), want (c0, nil)", output.Bytes(), err)
	}
}

func TestEncoderUnsupportedValues(t *testing.T) {
	tests := []struct {
		name  string
		value any
	}{
		{name: "nil"},
		{name: "bool", value: true},
		{name: "integer", value: int64(42)},
		{name: "string", value: "id"},
		{name: "bytes", value: []byte{1, 2}},
		{name: "array", value: [1]int{42}},
		{name: "slice", value: []int{42}},
		{name: "map", value: map[string]int{"id": 42}},
		{name: "struct", value: struct{ ID int }{ID: 42}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				output = &encoderTestWriter{}
				e      = NewEncoder(output)
				value  = &encoderTestEncodee{encode: func(w Writer) (err error) { return w.EncodeBool(true) }}
				err    = e.Encode(tt.value)
			)

			if err != ErrUnsupportedType || output.writes != 0 {
				t.Fatalf("Encode(%T) = %v, writes %d", tt.value, err, output.writes)
			}

			if err = e.Encode(value); err != nil || value.calls != 1 || !bytes.Equal(output.data, []byte{0xc3}) {
				t.Fatalf("Encode after unsupported value = (%x, %v), dispatches %d", output.data, err, value.calls)
			}
		})
	}
}

func TestEncoderFailuresAreTerminal(t *testing.T) {
	var (
		customFailure      = errors.New("test: custom encoding failure")
		destinationFailure = errors.New("test: destination failure")
		tests              = []struct {
			name       string
			encode     func(w Writer) (err error)
			limit      int
			limited    bool
			outputErr  error
			wantErr    error
			wantWire   []byte
			wantWrites int
		}{
			{
				name: "custom before output",
				encode: func(w Writer) (err error) {
					return customFailure
				},
				wantErr: customFailure,
			},
			{
				name: "custom after output",
				encode: func(w Writer) (err error) {
					if err = w.EncodeArrayHeader(1); err != nil {
						return err
					}

					return customFailure
				},
				wantErr: customFailure, wantWire: []byte{0x91}, wantWrites: 1,
			},
			{
				name: "destination failure",
				encode: func(w Writer) (err error) {
					return w.EncodeUint64(128)
				},
				limited: true, outputErr: destinationFailure, wantErr: destinationFailure, wantWrites: 1,
			},
			{
				name: "short write",
				encode: func(w Writer) (err error) {
					return w.EncodeUint64(128)
				},
				limited: true, limit: 1, wantErr: io.ErrShortWrite, wantWire: []byte{0xcc}, wantWrites: 1,
			},
			{
				name: "ignored destination failure",
				encode: func(w Writer) (err error) {
					_ = w.EncodeUint64(128)
					return nil
				},
				limited: true, outputErr: destinationFailure, wantErr: destinationFailure, wantWrites: 1,
			},
			{
				name: "destination failure before custom failure",
				encode: func(w Writer) (err error) {
					_ = w.EncodeUint64(128)
					return customFailure
				},
				limited: true, outputErr: destinationFailure, wantErr: destinationFailure, wantWrites: 1,
			},
			{
				name: "source failure",
				encode: func(w Writer) (err error) {
					_, err = w.EncodeStringFrom(2, strings.NewReader("a"))
					return err
				},
				wantErr: io.ErrUnexpectedEOF, wantWire: []byte{0xa2, 'a'}, wantWrites: 2,
			},
		}
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				output = &encoderTestWriter{limited: tt.limited, limit: tt.limit, err: tt.outputErr}
				e      = NewEncoder(output)
				value  = &encoderTestEncodee{encode: tt.encode}
				later  = &encoderTestEncodee{encode: func(w Writer) (err error) { return w.EncodeNil() }}
				first  = e.Encode(value)
				err    error
			)

			if !errors.Is(first, tt.wantErr) || first == tt.wantErr {
				t.Fatalf("Encode error = %v, want context wrapping %v", first, tt.wantErr)
			}

			if err = e.Encode(later); err != first || later.calls != 0 {
				t.Fatalf("later Encode = %v, dispatches %d, want stored error without dispatch", err, later.calls)
			}

			if output.writes != tt.wantWrites || !bytes.Equal(output.data, tt.wantWire) {
				t.Fatalf("failed output = (%x, %d writes), want (%x, %d)", output.data, output.writes, tt.wantWire, tt.wantWrites)
			}

			for count := 0; count < 2; count++ {
				if err = e.Close(); err != first {
					t.Fatalf("Close = %v, want stored error %v", err, first)
				}
			}
			if output.flushes != 0 || output.closes != 0 {
				t.Fatal("failed Encoder flushed or closed its output")
			}
		})
	}
}

func TestEncoderCloseLeavesOutputOpen(t *testing.T) {
	var (
		output = &encoderTestWriter{}
		e      = NewEncoder(output)
		value  = &encoderTestEncodee{encode: func(w Writer) (err error) { return w.EncodeBool(true) }}
		err    = e.Encode(value)
		n      int
	)

	if err != nil {
		t.Fatal(err)
	}

	for count := 0; count < 2; count++ {
		if err = e.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err = e.Encode(value); err != ErrClosed || value.calls != 1 || output.writes != 1 {
		t.Fatalf("Encode after Close = %v, dispatches %d, writes %d", err, value.calls, output.writes)
	}

	if output.flushes != 0 || output.closes != 0 {
		t.Fatal("Close flushed or closed the caller-owned output")
	}

	n, err = output.Write([]byte{0xc0})
	if err != nil || n != 1 || !bytes.Equal(output.data, []byte{0xc3, 0xc0}) {
		t.Fatalf("caller output after Close = (%x, %d, %v)", output.data, n, err)
	}
}

func TestEncoderUninitialized(t *testing.T) {
	tests := []struct {
		name  string
		build func() (e *Encoder)
	}{
		{name: "zero value", build: func() (e *Encoder) { return &Encoder{} }},
		{name: "nil constructor", build: func() (e *Encoder) { return NewEncoder(nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				e     = tt.build()
				value = &encoderTestEncodee{encode: func(w Writer) (err error) { return w.EncodeNil() }}
				err   = e.Encode(value)
			)

			if err != ErrUninitialized || value.calls != 0 {
				t.Fatalf("unattached Encode = %v, dispatches %d", err, value.calls)
			}

			if err = e.Close(); err != nil {
				t.Fatal(err)
			}

			if err = e.Encode(value); err != ErrClosed || value.calls != 0 {
				t.Fatalf("closed Encode = %v, dispatches %d", err, value.calls)
			}
		})
	}
}

func TestEncoderPublicMethods(t *testing.T) {
	var (
		typ  = reflect.TypeOf(&Encoder{})
		want = []string{"Close", "Encode"}
	)

	if typ.NumMethod() != len(want) {
		t.Fatalf("Encoder exposes %d methods, want %v", typ.NumMethod(), want)
	}

	for index, name := range want {
		if typ.Method(index).Name != name {
			t.Fatalf("Encoder method %d = %s, want %s", index, typ.Method(index).Name, name)
		}
	}
}
