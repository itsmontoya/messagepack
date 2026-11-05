package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
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
		{name: "bool value", value: true},
		{name: "integer value", value: int64(42)},
		{name: "string value", value: "id"},
		{name: "bytes value", value: []byte{1, 2}},
		{name: "nil pointer", value: (*bool)(nil)},
		{name: "channel", value: new(chan int)},
		{name: "function", value: new(func())},
		{name: "complex", value: new(complex64)},
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

func TestDecoderReflection(t *testing.T) {
	type identifier uint16
	type octet byte
	tests := []struct {
		name string
		wire []byte
		want any
	}{
		{name: "bool", wire: []byte{0xc3}, want: true},
		{name: "int", wire: []byte{0xff}, want: int(-1)},
		{name: "int8", wire: []byte{0xff}, want: int8(-1)},
		{name: "int16", wire: []byte{0xff}, want: int16(-1)},
		{name: "int32", wire: []byte{0xff}, want: int32(-1)},
		{name: "int64", wire: []byte{0xff}, want: int64(-1)},
		{name: "uint", wire: []byte{42}, want: uint(42)},
		{name: "uint8", wire: []byte{42}, want: uint8(42)},
		{name: "uint16", wire: []byte{42}, want: uint16(42)},
		{name: "uint32", wire: []byte{42}, want: uint32(42)},
		{name: "uint64", wire: []byte{42}, want: uint64(42)},
		{name: "uintptr", wire: []byte{42}, want: uintptr(42)},
		{name: "named integer", wire: []byte{42}, want: identifier(42)},
		{name: "float32", wire: []byte{0xca, 0x3f, 0xc0, 0, 0}, want: float32(1.5)},
		{name: "float64", wire: []byte{0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}, want: 1.5},
		{name: "widen float32", wire: []byte{0xca, 0x3f, 0xc0, 0, 0}, want: 1.5},
		{name: "string", wire: []byte{0xa2, 'i', 'd'}, want: "id"},
		{name: "bytes", wire: []byte{0xc4, 2, 1, 2}, want: []byte{1, 2}},
		{name: "named bytes", wire: []byte{0xc4, 2, 1, 2}, want: []octet{1, 2}},
		{name: "empty bytes", wire: []byte{0xc4, 0}, want: []byte{}},
		{name: "nil bytes", wire: []byte{0xc0}, want: []byte(nil)},
		{name: "array", wire: []byte{0x92, 1, 0xff}, want: [2]int{1, -1}},
		{name: "slice", wire: []byte{0x92, 1, 0xff}, want: []int{1, -1}},
		{name: "nil slice", wire: []byte{0xc0}, want: []int(nil)},
		{name: "empty slice", wire: []byte{0x90}, want: []int{}},
		{name: "map", wire: []byte{0x81, 0xa2, 'i', 'd', 42}, want: map[string]int{"id": 42}},
		{name: "nil map", wire: []byte{0xc0}, want: map[string]int(nil)},
		{name: "empty map", wire: []byte{0x80}, want: map[string]int{}},
		{name: "struct", wire: []byte{0x81, 0xa2, 'I', 'D', 42}, want: struct{ ID int }{ID: 42}},
		{name: "nested", wire: []byte{0x91, 0x81, 0xa2, 'o', 'k', 0x91, 0xc3}, want: []map[string][]bool{{"ok": {true}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				input  = &decoderTestReader{data: append(append([]byte{}, tt.wire...), 0xc3), maxRead: 1, endErr: io.EOF}
				d      = NewDecoder(input)
				target = reflect.New(reflect.TypeOf(tt.want))
				tail   bool
				err    = d.Decode(target.Interface())
			)

			if err != nil || !reflect.DeepEqual(target.Elem().Interface(), tt.want) || input.position != len(tt.wire) {
				t.Fatalf("Decode = (%#v, %v), position %d, want %#v", target.Elem().Interface(), err, input.position, tt.want)
			}

			if err = d.Decode(&tail); err != nil || !tail || input.position != len(input.data) {
				t.Fatalf("Decode next = (%v, %v), position %d", tail, err, input.position)
			}
		})
	}
}

func TestDecoderReflectionBytesReuse(t *testing.T) {
	tests := []struct {
		name       string
		length     int
		capacity   int
		nilInitial bool
		wire       []byte
		want       []byte
		reuse      bool
	}{
		{name: "grow nil", nilInitial: true, wire: []byte{0xc4, 2, 1, 2}, want: []byte{1, 2}},
		{name: "grow small", length: 1, capacity: 1, wire: []byte{0xc4, 2, 1, 2}, want: []byte{1, 2}},
		{name: "same length", length: 2, capacity: 2, wire: []byte{0xc4, 2, 1, 2}, want: []byte{1, 2}, reuse: true},
		{name: "extend length", length: 1, capacity: 8, wire: []byte{0xc4, 2, 1, 2}, want: []byte{1, 2}, reuse: true},
		{name: "shorten length", length: 4, capacity: 8, wire: []byte{0xc4, 2, 1, 2}, want: []byte{1, 2}, reuse: true},
		{name: "zero length capacity", capacity: 8, wire: []byte{0xc4, 2, 1, 2}, want: []byte{1, 2}, reuse: true},
		{name: "empty binary retains capacity", length: 4, capacity: 8, wire: []byte{0xc4, 0}, want: []byte{}, reuse: true},
		{name: "empty binary from nil", nilInitial: true, wire: []byte{0xc4, 0}, want: []byte{}},
		{name: "nil clears capacity", length: 4, capacity: 8, wire: []byte{0xc0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var initial = make([]byte, tt.length, tt.capacity)
			if tt.nilInitial {
				initial = nil
			}

			var storage = initial[:cap(initial)]
			for index := range storage {
				storage[index] = 9
			}
			var (
				before = append([]byte(nil), storage...)
				value  = struct{ Payload []byte }{Payload: initial}
				wire   = append([]byte{0x81, 0xa7, 'P', 'a', 'y', 'l', 'o', 'a', 'd'}, tt.wire...)
				input  = &decoderTestReader{data: append(wire, 0xc3), maxRead: 1, endErr: io.EOF}
				d      = NewDecoder(input)
				tail   bool
				err    = d.Decode(&value)
			)

			if err != nil || !reflect.DeepEqual(value.Payload, tt.want) || input.position != len(wire) {
				t.Fatalf("Decode = (%x, %v), position %d, want %x", value.Payload, err, input.position, tt.want)
			}

			if tt.reuse {
				if cap(value.Payload) != cap(initial) || &value.Payload[:cap(value.Payload)][0] != &storage[0] {
					t.Fatal("Decode replaced sufficient destination capacity")
				}

				copy(before, tt.want)
			} else if len(value.Payload) > 0 && len(storage) > 0 && &value.Payload[0] == &storage[0] {
				t.Fatal("Decode reused insufficient capacity")
			}

			if !bytes.Equal(storage, before) {
				t.Fatalf("original backing array = %x, want %x", storage, before)
			}

			if tt.want == nil && cap(value.Payload) != 0 {
				t.Fatal("wire nil retained destination capacity")
			}

			if err = d.Decode(&tail); err != nil || !tail {
				t.Fatalf("Decode next = (%v, %v)", tail, err)
			}
		})
	}
}

func TestDecoderReflectionNamedBytesReuse(t *testing.T) {
	type octet byte
	type payload []octet
	var (
		value   = make(payload, 1, 8)
		backing = &value[0]
		err     = NewDecoder(bytes.NewReader([]byte{0xc4, 2, 1, 2})).Decode(&value)
	)

	if err != nil || !reflect.DeepEqual(value, payload{1, 2}) || cap(value) != 8 || &value[0] != backing {
		t.Fatalf("Decode named bytes = (%v, %v), capacity %d", value, err, cap(value))
	}
}

func TestDecoderReflectionBytesReuseAliases(t *testing.T) {
	var (
		firstWire = []byte{0x81, 0xa7, 'P', 'a', 'y', 'l', 'o', 'a', 'd', 0xc4, 2, 1, 2}
		wire      = append(append([]byte(nil), firstWire...), firstWire...)
		value     = struct{ Payload []byte }{Payload: make([]byte, 0, 8)}
		d         = NewDecoder(bytes.NewReader(wire))
		err       error
	)

	wire[len(wire)-2], wire[len(wire)-1] = 3, 4
	if err = d.Decode(&value); err != nil {
		t.Fatal(err)
	}

	var alias = value.Payload
	wire[len(firstWire)-2], wire[len(firstWire)-1] = 0, 0
	if !bytes.Equal(alias, []byte{1, 2}) {
		t.Fatal("decoded bytes alias input storage")
	}

	if err = d.Decode(&value); err != nil || !bytes.Equal(value.Payload, []byte{3, 4}) || !bytes.Equal(alias, value.Payload) {
		t.Fatalf("repeated Decode = (%x, %v), retained alias %x", value.Payload, err, alias)
	}

	if &value.Payload[0] != &alias[0] {
		t.Fatal("repeated Decode replaced destination storage")
	}
}

func TestDecoderReflectionBytesReuseFailures(t *testing.T) {
	var (
		sourceFailure = errors.New("test: reused bytes source failure")
		tests         = []struct {
			name    string
			wire    []byte
			limit   uint64
			endErr  error
			wantErr error
			changed []byte
		}{
			{name: "limit before writes", wire: []byte{0xc4, 4, 1, 2, 3, 4}, limit: 3, wantErr: ErrLimitExceeded},
			{name: "wrong family", wire: []byte{0xa0}, wantErr: ErrTypeMismatch},
			{name: "truncated header", wire: []byte{0xc5, 0}, wantErr: io.ErrUnexpectedEOF},
			{name: "truncated reused body", wire: []byte{0xc4, 4, 1, 2}, wantErr: io.ErrUnexpectedEOF, changed: []byte{1, 2}},
			{name: "truncated grown body", wire: []byte{0xc4, 9, 1, 2}, wantErr: io.ErrUnexpectedEOF},
			{name: "source failure", wire: []byte{0xc4, 4, 1, 2, 3, 4}, endErr: sourceFailure, wantErr: sourceFailure, changed: []byte{1, 2, 3, 4}},
		}
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				storage = bytes.Repeat([]byte{9}, 8)
				value   = storage[:3]
				input   = &decoderTestReader{data: tt.wire, maxRead: 1, endErr: tt.endErr}
				d       = NewDecoder(input)
				err     error
			)

			if err = d.reflected.reader.SetLimits(ReaderLimits{MaxPayloadBytes: tt.limit}); err != nil {
				t.Fatal(err)
			}

			var (
				first = d.Decode(&value)
				reads = input.reads
				want  = bytes.Repeat([]byte{9}, 8)
			)

			copy(want, tt.changed)
			if !errors.Is(first, tt.wantErr) || len(value) != 3 || cap(value) != 8 || &value[0] != &storage[0] {
				t.Fatalf("Decode = (%x, %v), capacity %d, want %v without replacing the slice", value, first, cap(value), tt.wantErr)
			}

			if !bytes.Equal(storage, want) {
				t.Fatalf("backing array after failure = %x, want %x", storage, want)
			}

			if err = d.Decode(&value); err != first || input.reads != reads {
				t.Fatalf("terminal Decode = %v, reads %d, want (%v, %d)", err, input.reads, first, reads)
			}
		})
	}
}

func TestDecoderReflectionBytesReuseAllocations(t *testing.T) {
	for _, size := range []int{2, 1024, 65536} {
		t.Run(fmt.Sprintf("Bytes%d", size), func(t *testing.T) {
			var (
				value  = struct{ Payload []byte }{Payload: bytes.Repeat([]byte{7}, size)}
				output bytes.Buffer
				err    error
			)

			if err = NewEncoder(&output).Encode(&value); err != nil {
				t.Fatal(err)
			}

			var (
				input = bytes.NewReader(output.Bytes())
				d     = NewDecoder(input)
			)

			if err = d.Decode(&value); err != nil {
				t.Fatal(err)
			}

			var allocs = testing.AllocsPerRun(100, func() {
				input.Reset(output.Bytes())
				err = d.Decode(&value)
			})
			if err != nil || allocs != 0 {
				t.Fatalf("reused bytes Decode = %v, %.0f allocations, want zero", err, allocs)
			}

			if len(value.Payload) != size || value.Payload[0] != 7 || value.Payload[size-1] != 7 || input.Len() != 0 {
				t.Fatalf("reused payload length %d, remaining %d", len(value.Payload), input.Len())
			}
		})
	}
}

func TestDecoderReflectionInterface(t *testing.T) {
	tests := []struct {
		name string
		wire []byte
		want any
	}{
		{name: "nil", wire: []byte{0xc0}},
		{name: "bool", wire: []byte{0xc3}, want: true},
		{name: "unsigned", wire: []byte{42}, want: uint64(42)},
		{name: "signed", wire: []byte{0xff}, want: int64(-1)},
		{name: "positive signed", wire: []byte{0xd0, 42}, want: int64(42)},
		{name: "maximum unsigned", wire: []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, want: uint64(math.MaxUint64)},
		{name: "float32", wire: []byte{0xca, 0x3f, 0xc0, 0, 0}, want: float32(1.5)},
		{name: "float64", wire: []byte{0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}, want: 1.5},
		{name: "string", wire: []byte{0xa2, 'i', 'd'}, want: "id"},
		{name: "binary", wire: []byte{0xc4, 2, 1, 2}, want: []byte{1, 2}},
		{name: "array", wire: []byte{0x92, 1, 0xff}, want: []any{uint64(1), int64(-1)}},
		{name: "map", wire: []byte{0x81, 0xa2, 'i', 'd', 42}, want: map[any]any{"id": uint64(42)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				value any
				err   = NewDecoder(bytes.NewReader(tt.wire)).Decode(&value)
			)

			if err != nil || !reflect.DeepEqual(value, tt.want) {
				t.Fatalf("Decode interface = (%#v, %v), want %#v", value, err, tt.want)
			}
		})
	}
}

func TestDecoderReflectionPointers(t *testing.T) {
	var (
		value **int
		d     = NewDecoder(bytes.NewReader([]byte{42, 0xc0}))
		err   = d.Decode(&value)
	)

	if err != nil || value == nil || *value == nil || **value != 42 {
		t.Fatalf("Decode pointer chain = (%v, %v)", value, err)
	}

	if err = d.Decode(&value); err != nil || value != nil {
		t.Fatalf("Decode nil pointer = (%v, %v)", value, err)
	}
}

func TestDecoderReflectionStructFields(t *testing.T) {
	var (
		value = struct {
			hidden int
			ID     int
		}{ID: 1, hidden: 9}
		wire = []byte{0x84, 0xa2, 'I', 'D', 2, 0xa1, 'x', 0x91, 0x81, 0xa1, 'y', 0xc3,
			0xa6, 'h', 'i', 'd', 'd', 'e', 'n', 0, 0xa2, 'I', 'D', 42, 0xc3}
		input = bytes.NewReader(wire)
		d     = NewDecoder(input)
		err   = d.Decode(&value)
	)

	if err != nil || value.ID != 42 || value.hidden != 9 || input.Len() != 1 {
		t.Fatalf("Decode struct = (%+v, %v), remaining %d", value, err, input.Len())
	}
}

func TestDecoderReflectionNestedHook(t *testing.T) {
	var (
		value = struct{ Value *decoderTestDecodee }{
			Value: &decoderTestDecodee{decode: func(r Reader) (err error) { return r.Skip() }},
		}
		wire = []byte{0x81, 0xa5, 'V', 'a', 'l', 'u', 'e', 0x91, 42}
		err  = NewDecoder(bytes.NewReader(wire)).Decode(&value)
	)

	if err != nil || value.Value.calls != 1 {
		t.Fatalf("Decode nested hook = %v, calls %d", err, value.Value.calls)
	}
}

func TestDecoderReflectionCachedInterfaceHooks(t *testing.T) {
	var (
		hook      = &decoderTestDecodee{decode: func(r Reader) (err error) { return r.Skip() }}
		value any = uint64(0)
		d         = NewDecoder(bytes.NewReader([]byte{42, 0x91, 1, 0xc0}))
		err   error
	)

	if err = d.Decode(&value); err != nil || value != uint64(42) {
		t.Fatalf("Decode scalar interface = (%v, %v)", value, err)
	}

	value = hook
	if err = d.Decode(&value); err != nil || hook.calls != 1 || value != hook {
		t.Fatalf("Decode cached interface hook = (%v, %v), calls %d", value, err, hook.calls)
	}

	value = (*decoderTestDecodee)(nil)
	if err = d.Decode(&value); err != nil || value != (*decoderTestDecodee)(nil) {
		t.Fatalf("Decode cached typed nil interface hook = (%v, %v)", value, err)
	}
}

func TestDecoderReflectionWarmedAllocations(t *testing.T) {
	var (
		value = struct {
			ID     uint64
			Count  int64
			Active bool
			Ratio  float64
		}{ID: 42, Count: -3, Active: true, Ratio: 1.5}
		output bytes.Buffer
		err    error
	)

	if err = NewEncoder(&output).Encode(&value); err != nil {
		t.Fatal(err)
	}

	var (
		input = bytes.NewReader(output.Bytes())
		d     = NewDecoder(input)
	)

	if err = d.Decode(&value); err != nil {
		t.Fatal(err)
	}

	var allocs = testing.AllocsPerRun(100, func() {
		input.Reset(output.Bytes())
		err = d.Decode(&value)
	})
	if err != nil || allocs != 0 {
		t.Fatalf("warmed struct Decode = %v, %.0f allocations, want zero", err, allocs)
	}

	if value.ID != 42 || value.Count != -3 || !value.Active || value.Ratio != 1.5 || input.Len() != 0 {
		t.Fatalf("warmed struct Decode = %+v, remaining %d", value, input.Len())
	}
}

func TestDecoderReflectionWarmedStringAllocations(t *testing.T) {
	for _, length := range []int{3, 10, 1024, payloadBufferSize} {
		t.Run(fmt.Sprintf("Bytes%d", length), func(t *testing.T) {
			var (
				want   = strings.Repeat("x", length)
				value  = struct{ Name string }{Name: want}
				output bytes.Buffer
				err    error
			)

			if err = NewEncoder(&output).Encode(&value); err != nil {
				t.Fatal(err)
			}

			var (
				input  = bytes.NewReader(output.Bytes())
				d      = NewDecoder(input)
				allocs = testing.AllocsPerRun(100, func() {
					input.Reset(output.Bytes())
					err = d.Decode(&value)
				})
			)

			if err != nil || allocs > 1 || value.Name != want || input.Len() != 0 {
				t.Fatalf("warmed string Decode = %v, %g allocations, remaining %d", err, allocs, input.Len())
			}
		})
	}
}

func TestDecoderReflectionVisitBudgetPerRoot(t *testing.T) {
	var (
		value  [maximumReflectionVisits - 1]bool
		output bytes.Buffer
		writer = NewWriter(&output)
		err    error
	)

	if err = writer.EncodeArrayHeader(uint32(len(value))); err != nil {
		t.Fatal(err)
	}

	var (
		wire  = append(output.Bytes(), bytes.Repeat([]byte{0xc3}, len(value))...)
		input = bytes.NewReader(bytes.Repeat(wire, 2))
		d     = NewDecoder(input)
	)

	for index := 0; index < 2; index++ {
		if err = d.Decode(&value); err != nil || !value[0] || !value[len(value)-1] {
			t.Fatalf("Decode root %d = %v", index, err)
		}
	}
	if err = d.Decode(&value); !errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Decode after complete roots = %v, want clean EOF", err)
	}
}

func TestDecoderReflectionAllocatesHookPointer(t *testing.T) {
	var (
		value   **applicationMessage
		fixture = applicationMessageFixture()
		input   = bytes.NewReader(fixture)
		err     = NewDecoder(input).Decode(&value)
	)

	if err != nil || value == nil || *value == nil || (**value).ID != 7 || input.Len() != 0 {
		t.Fatalf("Decode pointer hook = (%v, %v), remaining %d", value, err, input.Len())
	}
}

func TestDecoderReflectionSourceFailure(t *testing.T) {
	var (
		sourceFailure = errors.New("test: reflection source failure")
		input         = &decoderTestReader{data: []byte{0xc3}, endErr: sourceFailure}
		d             = NewDecoder(input)
		value         bool
		first         = d.Decode(&value)
		reads         = input.reads
		err           error
	)

	if !errors.Is(first, sourceFailure) || first == sourceFailure || value {
		t.Fatalf("Decode = (%v, %v), want wrapped source failure", value, first)
	}

	if err = d.Decode(&value); err != first || input.reads != reads {
		t.Fatalf("terminal Decode = %v, reads %d", err, input.reads)
	}
}

func TestDecoderReflectionIgnoredHookEOF(t *testing.T) {
	var (
		value = struct{ Value decoderTestDecodee }{
			Value: decoderTestDecodee{decode: func(r Reader) (err error) {
				_ = r.DecodeNil()
				return nil
			}},
		}
		wire = []byte{0x81, 0xa5, 'V', 'a', 'l', 'u', 'e'}
		err  = NewDecoder(bytes.NewReader(wire)).Decode(&value)
	)

	if !errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) || value.Value.calls != 1 {
		t.Fatalf("Decode ignored child EOF = %v, calls %d", err, value.Value.calls)
	}
}

func TestDecoderReflectionFailuresAreTerminal(t *testing.T) {
	tests := []struct {
		name    string
		wire    []byte
		target  any
		wantErr error
	}{
		{name: "signed overflow", wire: []byte{0xcc, 128}, target: new(int8), wantErr: ErrOverflow},
		{name: "unsigned overflow", wire: []byte{0xcd, 1, 0}, target: new(uint8), wantErr: ErrOverflow},
		{name: "negative unsigned", wire: []byte{0xff}, target: new(uint64), wantErr: ErrOverflow},
		{name: "unsigned to signed overflow", wire: []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, target: new(int64), wantErr: ErrOverflow},
		{name: "float narrowing", wire: []byte{0xcb}, target: new(float32), wantErr: ErrTypeMismatch},
		{name: "family mismatch", wire: []byte{0xa0}, target: new(bool), wantErr: ErrTypeMismatch},
		{name: "array length", wire: []byte{0x92}, target: new([1]int), wantErr: ErrTypeMismatch},
		{name: "invalid map key", wire: []byte{0x81, 0x90}, target: new(any), wantErr: ErrTypeMismatch},
		{name: "reserved", wire: []byte{0xc1}, target: new(any), wantErr: ErrInvalidFormat},
		{name: "extension", wire: []byte{0xd4}, target: new(any), wantErr: ErrUnsupportedType},
		{name: "hostile slice count", wire: []byte{0xdd, 0xff, 0xff, 0xff, 0xff}, target: new([]int), wantErr: ErrLimitExceeded},
		{name: "hostile map count", wire: []byte{0xdf, 0xff, 0xff, 0xff, 0xff}, target: new(map[string]int), wantErr: ErrLimitExceeded},
		{name: "oversized payload", wire: []byte{0xc6, 1, 0, 0, 1}, target: new([]byte), wantErr: ErrLimitExceeded},
		{name: "missing array child", wire: []byte{0x91}, target: new([]int), wantErr: io.ErrUnexpectedEOF},
		{name: "missing map key", wire: []byte{0x81}, target: new(map[string]int), wantErr: io.ErrUnexpectedEOF},
		{name: "missing map value", wire: []byte{0x81, 0xa0}, target: new(map[string]int), wantErr: io.ErrUnexpectedEOF},
		{name: "truncated integer", wire: []byte{0xcd, 1}, target: new(int), wantErr: io.ErrUnexpectedEOF},
		{name: "empty input", target: new(int), wantErr: io.EOF},
		{name: "depth", wire: bytes.Repeat([]byte{0x91}, maximumReflectionDepth+1), target: new(any), wantErr: ErrLimitExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				input = &decoderTestReader{data: tt.wire, maxRead: 1, endErr: io.EOF}
				d     = NewDecoder(input)
				first = d.Decode(tt.target)
				reads = input.reads
				tail  bool
				err   error
			)

			if !errors.Is(first, tt.wantErr) {
				t.Fatalf("Decode = %v, want %v", first, tt.wantErr)
			}

			if tt.wantErr == io.ErrUnexpectedEOF && errors.Is(first, io.EOF) {
				t.Fatal("incomplete reflected value matches clean EOF")
			}

			if tt.wantErr != io.EOF && tt.wantErr != io.ErrUnexpectedEOF && first != tt.wantErr {
				t.Fatalf("Decode wrapped codec sentinel %v", first)
			}

			if err = d.Decode(&tail); err != first || input.reads != reads {
				t.Fatalf("terminal Decode = %v, reads %d", err, input.reads)
			}

			if err = d.Close(); err != first {
				t.Fatalf("Close = %v, want %v", err, first)
			}
		})
	}
}

func TestDecoderReflectionFloat32Bits(t *testing.T) {
	var (
		value float32
		err   = NewDecoder(bytes.NewReader([]byte{0xca, 0x7f, 0x80, 0, 1})).Decode(&value)
	)

	if err != nil || math.Float32bits(value) != 0x7f800001 {
		t.Fatalf("Decode signaling NaN = (%x, %v)", math.Float32bits(value), err)
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
