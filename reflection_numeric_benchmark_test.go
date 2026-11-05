package messagepack

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"testing"
	"testing/iotest"
)

// BenchmarkDecoderNumericCollections measures cached reflection through Decoder.Decode.
func BenchmarkDecoderNumericCollections(b *testing.B) {
	for _, encoding := range []string{"Int64Wide", "Uint64Wide", "Float64Wide", "Int64Compact", "Uint64Compact", "Int64Mixed", "Float64Widened"} {
		for _, size := range []int{1, 2, 16, 1024} {
			for _, shape := range []string{"Array", "Slice", "Struct"} {
				for _, input := range []string{"Memory", "Fragmented"} {
					b.Run(fmt.Sprintf("%s/Elements%d/%s/%s", encoding, size, shape, input), func(b *testing.B) {
						var (
							wire   bytes.Buffer
							writer     = NewWriter(&wire)
							typ        = reflect.TypeFor[int64]()
							body       = []byte{0xd3, 1, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef}
							want   any = int64(0x0123456789abcdef)
							err    error
						)

						if encoding == "Uint64Wide" || encoding == "Uint64Compact" {
							typ = reflect.TypeFor[uint64]()
							body[0] = 0xcf
							want = uint64(0x0123456789abcdef)
						} else if encoding == "Float64Wide" || encoding == "Float64Widened" {
							typ = reflect.TypeFor[float64]()
							body = []byte{0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0}
							want = float64(1.5)
						}

						if encoding == "Int64Compact" {
							body, want = []byte{7}, int64(7)
						} else if encoding == "Uint64Compact" {
							body, want = []byte{7}, uint64(7)
						} else if encoding == "Float64Widened" {
							body = []byte{0xca, 0x3f, 0xc0, 0, 0}
						}

						if shape == "Array" {
							typ = reflect.ArrayOf(size, typ)
						} else {
							typ = reflect.SliceOf(typ)
						}

						if shape == "Struct" {
							typ = reflect.StructOf([]reflect.StructField{{Name: "Values", Type: typ}})
							if err = writer.EncodeMapHeader(1); err != nil {
								b.Fatal(err)
							}

							if err = writer.EncodeString("Values"); err != nil {
								b.Fatal(err)
							}
						}

						if err = writer.EncodeArrayHeader(uint32(size)); err != nil {
							b.Fatal(err)
						}

						for index := 0; index < size; index++ {
							if encoding == "Int64Mixed" && index%2 == 0 {
								wire.WriteByte(7)
							} else {
								wire.Write(body)
							}
						}
						var (
							data             = wire.Bytes()
							source           = bytes.NewReader(data)
							reader io.Reader = source
							target           = reflect.New(typ)
						)

						if input == "Fragmented" {
							reader = iotest.OneByteReader(source)
						}

						var decoder = NewDecoder(reader)
						if err = decoder.Decode(target.Interface()); err != nil {
							b.Fatal(err)
						}

						b.ReportAllocs()
						b.SetBytes(int64(len(data)))
						b.ResetTimer()
						for iteration := 0; iteration < b.N; iteration++ {
							source.Reset(data)
							if err = decoder.Decode(target.Interface()); err != nil {
								b.Fatal(err)
							}
						}
						b.StopTimer()
						var values = target.Elem()
						if shape == "Struct" {
							values = values.Field(0)
						}

						for index := 0; index < size; index++ {
							var expected = want
							if encoding == "Int64Mixed" && index%2 == 0 {
								expected = int64(7)
							}

							if !reflect.DeepEqual(values.Index(index).Interface(), expected) {
								b.Fatalf("element %d = %v, want %v", index, values.Index(index).Interface(), expected)
							}
						}
						if source.Len() != 0 {
							b.Fatal("decode did not consume exactly one collection")
						}
					})
				}
			}
		}
	}
}
