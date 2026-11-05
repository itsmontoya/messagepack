package messagepack

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestDecoderNumericCollections(t *testing.T) {
	type signed int64
	type unsigned uint64
	type floating float64
	tests := []struct {
		name   string
		typ    reflect.Type
		wire   [][]byte
		values []any
	}{
		{name: "signed compact", typ: reflect.TypeFor[int64](), wire: [][]byte{{7}}, values: []any{int64(7)}},
		{name: "unsigned compact", typ: reflect.TypeFor[uint64](), wire: [][]byte{{7}}, values: []any{uint64(7)}},
		{name: "signed wide", typ: reflect.TypeFor[int64](), wire: [][]byte{{0xd3, 0x80, 0, 0, 0, 0, 0, 0, 0}}, values: []any{int64(math.MinInt64)}},
		{name: "unsigned wide", typ: reflect.TypeFor[uint64](), wire: [][]byte{{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}}, values: []any{uint64(math.MaxUint64)}},
		{name: "signed mixed", typ: reflect.TypeFor[int64](),
			wire:   [][]byte{{7}, {0xff}, {0xcc, 128}, {0xcd, 1, 0}, {0xce, 0, 1, 0, 0}, {0xcf, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, {0xd0, 0x80}, {0xd1, 0x80, 0}, {0xd2, 0x80, 0, 0, 0}, {0xd3, 0x80, 0, 0, 0, 0, 0, 0, 0}},
			values: []any{int64(7), int64(-1), int64(128), int64(256), int64(65536), int64(math.MaxInt64), int64(-128), int64(-32768), int64(math.MinInt32), int64(math.MinInt64)}},
		{name: "unsigned mixed", typ: reflect.TypeFor[uint64](),
			wire:   [][]byte{{7}, {0xcc, 128}, {0xcd, 1, 0}, {0xce, 0, 1, 0, 0}, {0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, {0xd0, 7}, {0xd1, 0, 7}, {0xd2, 0, 0, 0, 7}, {0xd3, 0, 0, 0, 0, 0, 0, 0, 7}},
			values: []any{uint64(7), uint64(128), uint64(256), uint64(65536), uint64(math.MaxUint64), uint64(7), uint64(7), uint64(7), uint64(7)}},
		{name: "float wide NaN", typ: reflect.TypeFor[float64](), wire: [][]byte{{0xcb, 0x7f, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}}, values: []any{math.Float64frombits(0x7ff0123456789abc)}},
		{name: "float mixed", typ: reflect.TypeFor[float64](),
			wire:   [][]byte{{0xca, 0x3f, 0xc0, 0, 0}, {0xcb, 0x80, 0, 0, 0, 0, 0, 0, 0}, {0xcb, 0xff, 0xf8, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc}, {0xca, 0x7f, 0x80, 0, 1}},
			values: []any{float64(1.5), math.Float64frombits(1 << 63), math.Float64frombits(0xfff8123456789abc), float64(math.Float32frombits(0x7f800001))}},
		{name: "named signed", typ: reflect.TypeFor[signed](), wire: [][]byte{{0xff}}, values: []any{signed(-1)}},
		{name: "named unsigned", typ: reflect.TypeFor[unsigned](), wire: [][]byte{{7}}, values: []any{unsigned(7)}},
		{name: "named float", typ: reflect.TypeFor[floating](), wire: [][]byte{{0xcb, 0x80, 0, 0, 0, 0, 0, 0, 0}}, values: []any{floating(math.Float64frombits(1 << 63))}},
	}

	for _, tt := range tests {
		for _, size := range []int{0, 1, 2, 15, 16, 17, 63, 64, 65, 257} {
			for _, shape := range []string{"Array", "Slice", "StructArray", "StructSlice"} {
				for _, input := range []string{"Reader", "Buffer", "Buffered", "Fragmented"} {
					t.Run(fmt.Sprintf("%s/%d/%s/%s", tt.name, size, shape, input), func(t *testing.T) {
						var (
							output bytes.Buffer
							writer = NewWriter(&output)
							typ    = reflect.SliceOf(tt.typ)
							err    error
						)

						if shape == "Array" || shape == "StructArray" {
							typ = reflect.ArrayOf(size, tt.typ)
						}

						if shape == "StructArray" || shape == "StructSlice" {
							typ = reflect.StructOf([]reflect.StructField{{Name: "Values", Type: typ}})
							output.Write([]byte{0x81, 0xa6, 'V', 'a', 'l', 'u', 'e', 's'})
						}

						if err = writer.EncodeArrayHeader(uint32(size)); err != nil {
							t.Fatal(err)
						}

						for index := 0; index < size; index++ {
							output.Write(tt.wire[index%len(tt.wire)])
						}
						var (
							length           = output.Len()
							data             = append(output.Bytes(), 0xc3)
							target           = reflect.New(typ)
							source, position = decoderNumericInput(t, data, input)
						)

						var decoder = NewDecoder(source)
						if err = decoder.Decode(target.Interface()); err != nil || position() != length {
							t.Fatalf("Decode = %v, position %d, want %d", err, position(), length)
						}

						var values = target.Elem()
						if values.Kind() == reflect.Struct {
							values = values.Field(0)
						}

						if values.Len() != size || values.Kind() == reflect.Slice && values.IsNil() {
							t.Fatalf("decoded length %d, want non-nil collection of length %d", values.Len(), size)
						}

						for index := 0; index < size; index++ {
							var (
								got  = values.Index(index)
								want = reflect.ValueOf(tt.values[index%len(tt.values)])
							)

							if tt.typ.Kind() == reflect.Float64 {
								if math.Float64bits(got.Float()) != math.Float64bits(want.Float()) {
									t.Fatalf("element %d bits = %x, want %x", index, math.Float64bits(got.Float()), math.Float64bits(want.Float()))
								}
							} else if !reflect.DeepEqual(got.Interface(), want.Interface()) {
								t.Fatalf("element %d = %v, want %v", index, got, want)
							}
						}
						var tail bool
						if err = decoder.Decode(&tail); err != nil || !tail {
							t.Fatalf("Decode tail = (%v, %v)", tail, err)
						}
					})
				}
			}
		}
	}
}

func TestDecoderNumericNamedCollections(t *testing.T) {
	type signedArray [2]int64
	type unsignedArray [2]uint64
	type floatArray [2]float64
	type signedSlice []int64
	type unsignedSlice []uint64
	type floatSlice []float64
	tests := []struct {
		name string
		want any
	}{
		{name: "signed array", want: signedArray{-1, 2}},
		{name: "unsigned array", want: unsignedArray{1, 2}},
		{name: "float array", want: floatArray{1.5, -2.5}},
		{name: "signed slice", want: signedSlice{-1, 2}},
		{name: "unsigned slice", want: unsignedSlice{1, 2}},
		{name: "float slice", want: floatSlice{1.5, -2.5}},
		{name: "nil signed slice", want: signedSlice(nil)},
		{name: "nil unsigned slice", want: unsignedSlice(nil)},
		{name: "nil float slice", want: floatSlice(nil)},
		{name: "empty signed slice", want: signedSlice{}},
		{name: "empty unsigned slice", want: unsignedSlice{}},
		{name: "empty float slice", want: floatSlice{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				wire   bytes.Buffer
				target = reflect.New(reflect.TypeOf(tt.want))
				err    error
			)

			if err = NewEncoder(&wire).Encode(tt.want); err != nil {
				t.Fatal(err)
			}

			if err = NewDecoder(&wire).Decode(target.Interface()); err != nil || !reflect.DeepEqual(target.Elem().Interface(), tt.want) {
				t.Fatalf("Decode = (%v, %v), want %v", target.Elem(), err, tt.want)
			}
		})
	}
}

func TestDecoderNumericCollectionFailures(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[int64](), reflect.TypeFor[uint64](), reflect.TypeFor[float64]()} {
		var (
			body     = []byte{0xd3, 0, 0, 0, 0, 0, 0, 0, 7}
			good any = int64(7)
		)

		if typ.Kind() == reflect.Uint64 {
			body[0], good = 0xcf, uint64(7)
		} else if typ.Kind() == reflect.Float64 {
			body = []byte{0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}
			good = float64(1.5)
		}

		type testcase struct {
			name     string
			bad      []byte
			consumed int
			wantErr  error
		}

		tests := []testcase{
			{name: "wrong family", bad: []byte{0xa1, 'x'}, consumed: 1, wantErr: ErrTypeMismatch},
			{name: "reserved", bad: []byte{0xc1, 7}, consumed: 1, wantErr: ErrInvalidFormat},
			{name: "extension", bad: []byte{0xd4, 0, 7}, consumed: 1, wantErr: ErrUnsupportedType},
			{name: "missing child", wantErr: io.ErrUnexpectedEOF},
			{name: "truncated child", bad: body[:5], consumed: 5, wantErr: io.ErrUnexpectedEOF},
		}

		if typ.Kind() == reflect.Int64 {
			tests = append(tests, testcase{name: "overflow", bad: []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 7}, consumed: 9, wantErr: ErrOverflow})
		} else if typ.Kind() == reflect.Uint64 {
			tests = append(tests, testcase{name: "negative", bad: []byte{0xff, 7}, consumed: 1, wantErr: ErrOverflow})
		}

		for _, tt := range tests {
			for _, prefix := range []int{0, 1, 2, 15, 16, 63, 64, 65} {
				for _, shape := range []string{"Array", "Slice"} {
					for _, input := range []string{"Reader", "Buffer", "Buffered", "Fragmented"} {
						t.Run(fmt.Sprintf("%s/%s/%d/%s/%s", typ, tt.name, prefix, shape, input), func(t *testing.T) {
							var (
								wire   = []byte{0xdc, 0, 67}
								target reflect.Value
								want   reflect.Value
							)

							for index := 0; index < prefix; index++ {
								wire = append(wire, body...)
							}
							wire = append(wire, tt.bad...)
							if shape == "Array" {
								target = reflect.New(reflect.ArrayOf(67, typ))
								want = reflect.New(target.Elem().Type()).Elem()
							} else {
								target = reflect.New(reflect.SliceOf(typ))
								target.Elem().Set(reflect.MakeSlice(target.Elem().Type(), 67, 100))
								want = reflect.MakeSlice(target.Elem().Type(), 67, 67)
							}

							for index := 0; index < 67; index++ {
								target.Elem().Index(index).Set(reflect.ValueOf(9).Convert(typ))
								want.Index(index).Set(target.Elem().Index(index))
								if shape == "Array" && index < prefix {
									want.Index(index).Set(reflect.ValueOf(good))
								}
							}
							var (
								alias            = target.Elem().Interface()
								source, position = decoderNumericInput(t, wire, input)
								decoder          = NewDecoder(source)
								first            = decoder.Decode(target.Interface())
								consumed         = 3 + prefix*len(body) + tt.consumed
								tail             bool
								err              error
							)

							if !errors.Is(first, tt.wantErr) || position() != consumed || !reflect.DeepEqual(target.Elem().Interface(), want.Interface()) {
								t.Fatalf("Decode = (%v, %v), position %d; want (%v, %v), position %d", target.Elem(), first, position(), want, tt.wantErr, consumed)
							}

							if tt.wantErr == io.ErrUnexpectedEOF && errors.Is(first, io.EOF) {
								t.Fatal("missing required child matches clean EOF")
							}

							if shape == "Slice" && !reflect.DeepEqual(alias, want.Interface()) {
								t.Fatal("failed slice decode changed an alias")
							}

							if err = decoder.Decode(&tail); err != first || position() != consumed {
								t.Fatalf("terminal Decode = %v, position %d", err, position())
							}

							if err = decoder.Close(); err != first {
								t.Fatalf("Close = %v, want %v", err, first)
							}
						})
					}
				}
			}
		}
	}
}

func decoderNumericInput(t *testing.T, data []byte, mode string) (source io.Reader, position func() int) {
	t.Helper()
	switch mode {
	case "Reader":
		var memory = bytes.NewReader(data)
		return memory, func() (n int) { return len(data) - memory.Len() }
	case "Buffer":
		var buffer = bytes.NewBuffer(data)
		return buffer, func() (n int) { return len(data) - buffer.Len() }
	case "Buffered":
		var buffered = bufio.NewReaderSize(bytes.NewReader(data), len(data))
		if _, err := buffered.Peek(len(data)); err != nil {
			t.Fatal(err)
		}

		return buffered, func() (n int) { return len(data) - buffered.Buffered() }
	default:
		var fragmented = &decoderTestReader{data: data, maxRead: 1, endErr: io.EOF}
		return fragmented, func() (n int) { return fragmented.position }
	}
}

func TestDecoderNumericScalarFallbacks(t *testing.T) {
	tests := []struct {
		name     string
		wire     []byte
		initial  any
		want     any
		wantErr  error
		consumed int
	}{
		{name: "narrow signed", wire: []byte{0x93, 7, 0xcd, 1, 0, 8}, initial: [3]int8{9, 9, 9}, want: [3]int8{7, 9, 9}, wantErr: ErrOverflow, consumed: 5},
		{name: "narrow unsigned", wire: []byte{0x93, 7, 0xcd, 1, 0, 8}, initial: [3]uint16{9, 9, 9}, want: [3]uint16{7, 256, 8}, consumed: 6},
		{name: "narrow unsigned overflow", wire: []byte{0x93, 7, 0xce, 0, 1, 0, 0, 8}, initial: [3]uint16{9, 9, 9}, want: [3]uint16{7, 9, 9}, wantErr: ErrOverflow, consumed: 7},
		{name: "float32 mismatch", wire: []byte{0x92, 0xca, 0x3f, 0xc0, 0, 0, 0xcb, 7}, initial: [2]float32{9, 9}, want: [2]float32{1.5, 9}, wantErr: ErrTypeMismatch, consumed: 7},
		{name: "pointers", wire: []byte{0x92, 7, 8}, initial: [2]*int64{}, want: [2]*int64{new(int64(7)), new(int64(8))}, consumed: 3},
		{name: "interfaces", wire: []byte{0x92, 7, 8}, initial: [2]any{}, want: [2]any{uint64(7), uint64(8)}, consumed: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				target = reflect.New(reflect.TypeOf(tt.initial))
				source = bytes.NewReader(tt.wire)
				err    error
			)

			target.Elem().Set(reflect.ValueOf(tt.initial))
			err = NewDecoder(source).Decode(target.Interface())
			if err != tt.wantErr || len(tt.wire)-source.Len() != tt.consumed || !reflect.DeepEqual(target.Elem().Interface(), tt.want) {
				t.Fatalf("Decode = (%v, %v), consumed %d; want (%v, %v), consumed %d", target.Elem(), err, len(tt.wire)-source.Len(), tt.want, tt.wantErr, tt.consumed)
			}
		})
	}
}

func TestDecoderNumericAdvertisedCounts(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[[]int64](), reflect.TypeFor[[]uint64](), reflect.TypeFor[[]float64](), reflect.TypeFor[[2]int64](), reflect.TypeFor[[2]uint64](), reflect.TypeFor[[2]float64]()} {
		for _, count := range []uint32{1, maximumReflectionVisits, math.MaxUint32} {
			t.Run(fmt.Sprintf("%s/%d", typ, count), func(t *testing.T) {
				var (
					wire                = []byte{0xdd, 0, 0, 0, 0, 0xc3}
					target              = reflect.New(typ)
					wantErr       error = ErrLimitExceeded
					wantRemaining       = 1
				)

				binary.BigEndian.PutUint32(wire[1:5], count)
				if count == 1 {
					wantErr = ErrTypeMismatch
					if typ.Kind() == reflect.Slice {
						wantRemaining = 0
					}
				}

				var (
					source = bytes.NewReader(wire)
					err    = NewDecoder(source).Decode(target.Interface())
				)

				if err != wantErr || source.Len() != wantRemaining {
					t.Fatalf("Decode = %v, remaining %d, want (%v, %d)", err, source.Len(), wantErr, wantRemaining)
				}
			})
		}
	}
	for _, typ := range []reflect.Type{reflect.TypeFor[[]int64](), reflect.TypeFor[[]uint64](), reflect.TypeFor[[]float64]()} {
		var (
			wire   = []byte{0xdd, 0, 0, 0, 0}
			target = reflect.New(typ)
			err    error
		)

		binary.BigEndian.PutUint32(wire[1:], maximumReflectionVisits-1)
		if err = NewDecoder(bytes.NewReader(wire)).Decode(target.Interface()); !errors.Is(err, io.ErrUnexpectedEOF) || !target.Elem().IsNil() {
			t.Fatalf("Decode allowed count without children for %s = (%v, %v)", typ, target.Elem(), err)
		}
	}
}

func TestDecoderNumericSlicePublication(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[int64](), reflect.TypeFor[uint64](), reflect.TypeFor[float64]()} {
		for _, form := range []string{"nil", "empty", "values"} {
			t.Run(typ.String()+"/"+form, func(t *testing.T) {
				var (
					target     = reflect.New(reflect.SliceOf(typ))
					storage    = reflect.MakeSlice(target.Elem().Type(), 8, 8)
					wire       = []byte{0x92, 7, 8}
					wantLength = 2
					err        error
				)

				for index := 0; index < storage.Len(); index++ {
					storage.Index(index).Set(reflect.ValueOf(9).Convert(typ))
				}
				target.Elem().Set(storage)
				if typ.Kind() == reflect.Float64 {
					wire = []byte{0x92, 0xca, 0x3f, 0xc0, 0, 0, 0xca, 0x40, 0, 0, 0}
				}

				if form == "nil" {
					wire, wantLength = []byte{0xc0}, 0
				} else if form == "empty" {
					wire, wantLength = []byte{0x90}, 0
				}

				if err = NewDecoder(bytes.NewReader(wire)).Decode(target.Interface()); err != nil || target.Elem().Len() != wantLength || target.Elem().IsNil() != (form == "nil") {
					t.Fatalf("Decode = (%v, %v)", target.Elem(), err)
				}

				for index := 0; index < storage.Len(); index++ {
					if !reflect.DeepEqual(storage.Index(index).Interface(), reflect.ValueOf(9).Convert(typ).Interface()) {
						t.Fatal("successful slice decode overwrote original storage")
					}
				}
			})
		}
	}
}

func TestDecoderNumericTransportPrecedence(t *testing.T) {
	var sourceFailure = errors.New("test: numeric source failure")
	for _, typ := range []reflect.Type{reflect.TypeFor[int64](), reflect.TypeFor[uint64](), reflect.TypeFor[float64]()} {
		for _, failure := range []string{"complete body", "reserved tag", "EOF"} {
			t.Run(typ.String()+"/"+failure, func(t *testing.T) {
				var (
					body       = []byte{0xd3, 0, 0, 0, 0, 0, 0, 0, 7}
					good   any = int64(7)
					wire       = []byte{0x92}
					target     = reflect.New(reflect.ArrayOf(2, typ))
					want       = reflect.New(target.Elem().Type()).Elem()
				)

				if typ.Kind() == reflect.Uint64 {
					body[0], good = 0xcf, uint64(7)
				} else if typ.Kind() == reflect.Float64 {
					body, good = []byte{0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}, float64(1.5)
				}

				wire = append(wire, body...)
				if failure == "reserved tag" {
					wire = append(wire, 0xc1)
				} else {
					wire = append(wire, body...)
				}

				for index := 0; index < 2; index++ {
					target.Elem().Index(index).Set(reflect.ValueOf(9).Convert(typ))
					want.Index(index).Set(reflect.ValueOf(good))
				}
				var (
					source  = &decoderTestReader{data: wire, maxRead: 1, endErr: sourceFailure}
					decoder = NewDecoder(source)
					first   error
					tail    bool
					err     error
				)

				if failure == "EOF" {
					source.endErr = io.EOF
				} else {
					want.Index(1).Set(reflect.ValueOf(9).Convert(typ))
				}

				first = decoder.Decode(target.Interface())
				if failure == "EOF" && first != nil || failure != "EOF" && !errors.Is(first, sourceFailure) || !reflect.DeepEqual(target.Elem().Interface(), want.Interface()) {
					t.Fatalf("Decode = (%v, %v), want %v", target.Elem(), first, want)
				}

				var reads = source.reads
				err = decoder.Decode(&tail)
				if failure == "EOF" {
					if !errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
						t.Fatalf("next root = %v, want clean EOF", err)
					}
				} else if err != first {
					t.Fatalf("terminal Decode = %v, want %v", err, first)
				}

				if source.reads != reads {
					t.Fatal("terminal or pending EOF performed additional I/O")
				}
			})
		}
	}
}

func TestDecoderNumericStructKeys(t *testing.T) {
	for _, input := range []string{"Reader", "Buffer", "Buffered", "Fragmented"} {
		t.Run(input, func(t *testing.T) {
			var (
				value = struct {
					First   int64
					Second  uint64
					Values  [2]float64
					Slice   []int64
					Missing int64
				}{Missing: 9}
				wire = []byte{0x86,
					0xa6, 'S', 'e', 'c', 'o', 'n', 'd', 2,
					0xa5, 'S', 'l', 'i', 'c', 'e', 0x92, 7, 8,
					0xa5, 'F', 'i', 'r', 's', 't', 0xff,
					0xa6, 'V', 'a', 'l', 'u', 'e', 's', 0x92, 0xca, 0x3f, 0xc0, 0, 0, 0xcb, 0x40, 4, 0, 0, 0, 0, 0, 0,
					0xa1, 'x', 0x90,
					0xa5, 'F', 'i', 'r', 's', 't', 42, 0xc3}
				source, position = decoderNumericInput(t, wire, input)
				err              = NewDecoder(source).Decode(&value)
			)

			if err != nil || position() != len(wire)-1 || value.First != 42 || value.Second != 2 || value.Values != [2]float64{1.5, 2.5} || !reflect.DeepEqual(value.Slice, []int64{7, 8}) || value.Missing != 9 {
				t.Fatalf("Decode keyed numeric fields = (%+v, %v), position %d", value, err, position())
			}
		})
	}
}

func TestDecoderNumericNestedFailure(t *testing.T) {
	var (
		value = struct {
			Array [2]int64
			Slice []float64
		}{Array: [2]int64{9, 9}, Slice: []float64{9, 9}}
		alias = value.Slice
		wire  = []byte{0x82, 0xa5, 'A', 'r', 'r', 'a', 'y', 0x92, 7, 8,
			0xa5, 'S', 'l', 'i', 'c', 'e', 0x92, 0xca, 0x3f, 0xc0, 0, 0}
		decoder = NewDecoder(bytes.NewReader(wire))
		err     = decoder.Decode(&value)
	)

	if !errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) || value.Array != [2]int64{7, 8} || !reflect.DeepEqual(value.Slice, []float64{9, 9}) || &value.Slice[0] != &alias[0] {
		t.Fatalf("Decode nested failure = (%+v, %v)", value, err)
	}
}

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
