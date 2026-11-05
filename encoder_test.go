package messagepack

import (
	"bytes"
	"errors"
	"io"
	"math"
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
		{name: "channel", value: make(chan int)},
		{name: "function", value: func() {}},
		{name: "complex64", value: complex64(1)},
		{name: "complex128", value: complex128(1)},
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

func TestEncoderReflection(t *testing.T) {
	type identifier uint16
	type octet byte
	type score float32
	var (
		integer = 42
		tests   = []struct {
			name  string
			value any
			wire  []byte
		}{
			{name: "nil", wire: []byte{0xc0}},
			{name: "bool", value: true, wire: []byte{0xc3}},
			{name: "int", value: -1, wire: []byte{0xff}},
			{name: "int8", value: int8(-1), wire: []byte{0xff}},
			{name: "int16", value: int16(-1), wire: []byte{0xff}},
			{name: "int32", value: int32(-1), wire: []byte{0xff}},
			{name: "int64", value: int64(-1), wire: []byte{0xff}},
			{name: "uint", value: uint(42), wire: []byte{42}},
			{name: "uint8", value: uint8(42), wire: []byte{42}},
			{name: "uint16", value: uint16(42), wire: []byte{42}},
			{name: "uint32", value: uint32(42), wire: []byte{42}},
			{name: "uint64", value: uint64(42), wire: []byte{42}},
			{name: "uintptr", value: uintptr(42), wire: []byte{42}},
			{name: "named integer", value: identifier(42), wire: []byte{42}},
			{name: "float32", value: float32(1.5), wire: []byte{0xca, 0x3f, 0xc0, 0, 0}},
			{name: "float32 signaling NaN", value: math.Float32frombits(0x7f800001), wire: []byte{0xca, 0x7f, 0x80, 0, 1}},
			{name: "addressable float32 signaling NaN", value: &struct{ F float32 }{F: math.Float32frombits(0x7f800001)}, wire: []byte{0x81, 0xa1, 'F', 0xca, 0x7f, 0x80, 0, 1}},
			{name: "addressable named float32 signaling NaN", value: &struct{ F score }{F: score(math.Float32frombits(0x7f800001))}, wire: []byte{0x81, 0xa1, 'F', 0xca, 0x7f, 0x80, 0, 1}},
			{name: "float64", value: 1.5, wire: []byte{0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}},
			{name: "string", value: "id", wire: []byte{0xa2, 'i', 'd'}},
			{name: "bytes", value: []byte{1, 2}, wire: []byte{0xc4, 2, 1, 2}},
			{name: "named bytes", value: []octet{1, 2}, wire: []byte{0xc4, 2, 1, 2}},
			{name: "nil bytes", value: []byte(nil), wire: []byte{0xc4, 0}},
			{name: "array", value: [2]int{1, -1}, wire: []byte{0x92, 1, 0xff}},
			{name: "slice", value: []int{1, -1}, wire: []byte{0x92, 1, 0xff}},
			{name: "nil slice", value: []int(nil), wire: []byte{0xc0}},
			{name: "empty slice", value: []int{}, wire: []byte{0x90}},
			{name: "map", value: map[string]int{"id": 42}, wire: []byte{0x81, 0xa2, 'i', 'd', 42}},
			{name: "nil map", value: map[string]int(nil), wire: []byte{0xc0}},
			{name: "empty map", value: map[string]int{}, wire: []byte{0x80}},
			{name: "pointer", value: &integer, wire: []byte{42}},
			{name: "nil pointer", value: (*int)(nil), wire: []byte{0xc0}},
			{name: "struct", value: struct {
				ID     int
				hidden chan int
			}{ID: 42}, wire: []byte{0x81, 0xa2, 'I', 'D', 42}},
			{name: "nested", value: []any{map[string][]bool{"ok": {true}}}, wire: []byte{0x91, 0x81, 0xa2, 'o', 'k', 0x91, 0xc3}},
		}
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				output bytes.Buffer
				err    = NewEncoder(&output).Encode(tt.value)
			)

			if err != nil || !bytes.Equal(output.Bytes(), tt.wire) {
				t.Fatalf("Encode(%T) = (%x, %v), want %x", tt.value, output.Bytes(), err, tt.wire)
			}
		})
	}
}

func TestEncoderReflectionNestedHook(t *testing.T) {
	var (
		output bytes.Buffer
		value  = &struct{ Value encoderTestEncodee }{
			Value: encoderTestEncodee{encode: func(w Writer) (err error) { return w.EncodeBool(true) }},
		}
		want = []byte{0x81, 0xa5, 'V', 'a', 'l', 'u', 'e', 0xc3}
		err  = NewEncoder(&output).Encode(value)
	)

	if err != nil || value.Value.calls != 1 || !bytes.Equal(output.Bytes(), want) {
		t.Fatalf("Encode nested hook = (%x, %v), calls %d", output.Bytes(), err, value.Value.calls)
	}
}

func TestEncoderReflectionCachedHookAddressability(t *testing.T) {
	var (
		value = struct{ Value encoderTestEncodee }{
			Value: encoderTestEncodee{encode: func(w Writer) (err error) { return w.EncodeBool(true) }},
		}
		output bytes.Buffer
		e      = NewEncoder(&output)
		prefix = []byte{0x81, 0xa5, 'V', 'a', 'l', 'u', 'e'}
		err    error
	)

	if err = e.Encode(value); err != nil || value.Value.calls != 0 || !bytes.Equal(output.Bytes(), append(prefix, 0x80)) {
		t.Fatalf("unaddressable hook field = (%x, %v), calls %d", output.Bytes(), err, value.Value.calls)
	}

	output.Reset()
	if err = e.Encode(&value); err != nil || value.Value.calls != 1 || !bytes.Equal(output.Bytes(), append(prefix, 0xc3)) {
		t.Fatalf("addressable cached hook field = (%x, %v), calls %d", output.Bytes(), err, value.Value.calls)
	}
}

func TestEncoderReflectionFailuresAreTerminal(t *testing.T) {
	var (
		cycle = make([]any, 1)
		tests = []struct {
			name    string
			value   any
			wantErr error
		}{
			{name: "unsupported child", value: []any{42, make(chan int)}, wantErr: ErrUnsupportedType},
			{name: "cycle", value: cycle, wantErr: ErrLimitExceeded},
		}
	)

	cycle[0] = cycle
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				output = &encoderTestWriter{}
				e      = NewEncoder(output)
				first  = e.Encode(tt.value)
				writes = output.writes
				err    error
			)

			if first != tt.wantErr || writes == 0 {
				t.Fatalf("Encode = %v, writes %d, want %v after output", first, writes, tt.wantErr)
			}

			if err = e.Encode(true); err != first || output.writes != writes {
				t.Fatalf("terminal Encode = %v, writes %d", err, output.writes)
			}

			if err = e.Close(); err != first {
				t.Fatalf("Close = %v, want %v", err, first)
			}
		})
	}
}

func TestEncoderReflectionDestinationFailure(t *testing.T) {
	var (
		destinationFailure = errors.New("test: reflection destination failure")
		output             = &encoderTestWriter{err: destinationFailure}
		e                  = NewEncoder(output)
		first              = e.Encode(true)
		writes             = output.writes
		err                error
	)

	if !errors.Is(first, destinationFailure) || first == destinationFailure {
		t.Fatalf("Encode = %v, want wrapped destination failure", first)
	}

	if err = e.Encode(true); err != first || output.writes != writes {
		t.Fatalf("terminal Encode = %v, writes %d", err, output.writes)
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
