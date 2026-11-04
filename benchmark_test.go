package messagepack

import (
	"bytes"
	"strings"
	"testing"
)

var (
	benchmarkBytesSink   []byte
	benchmarkStringSink  string
	benchmarkBoolSink    bool
	benchmarkIntSink     int64
	benchmarkUintSink    uint64
	benchmarkFloat32Sink float32
	benchmarkFloat64Sink float64
	benchmarkHeaderSink  uint32
	benchmarkCountSink   int
	benchmarkMessageSink applicationMessage
)

func BenchmarkReadPrimitives(b *testing.B) {
	var (
		nilWire     = []byte{0xc0}
		boolWire    = []byte{0xc3}
		intWire     = []byte{0xd2, 0xff, 0xff, 0x7f, 0xff}
		uintWire    = []byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
		float32Wire = []byte{0xca, 0x3f, 0xc0, 0x00, 0x00}
		float64Wire = []byte{0xcb, 0x3f, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
		tests       = []struct {
			name string
			size int
			run  func() (n int, err error)
		}{
			{name: "Nil", size: len(nilWire), run: func() (n int, err error) { return ReadNil(nilWire) }},
			{name: "Bool", size: len(boolWire), run: func() (n int, err error) {
				benchmarkBoolSink, n, err = ReadBool(boolWire)
				return n, err
			}},
			{name: "Int64", size: len(intWire), run: func() (n int, err error) {
				benchmarkIntSink, n, err = ReadInt64(intWire)
				return n, err
			}},
			{name: "Uint64", size: len(uintWire), run: func() (n int, err error) {
				benchmarkUintSink, n, err = ReadUint64(uintWire)
				return n, err
			}},
			{name: "Float32", size: len(float32Wire), run: func() (n int, err error) {
				benchmarkFloat32Sink, n, err = ReadFloat32(float32Wire)
				return n, err
			}},
			{name: "Float64", size: len(float64Wire), run: func() (n int, err error) {
				benchmarkFloat64Sink, n, err = ReadFloat64(float64Wire)
				return n, err
			}},
			{name: "Float64Widen", size: len(float32Wire), run: func() (n int, err error) {
				benchmarkFloat64Sink, n, err = ReadFloat64(float32Wire)
				return n, err
			}},
		}
	)
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) { benchmarkReads(b, tt.size, tt.run) })
	}
}

func BenchmarkHeaders(b *testing.B) {
	var (
		arrayWire = []byte{0xdd, 0x00, 0x01, 0x00, 0x00}
		mapWire   = []byte{0xdf, 0x00, 0x01, 0x00, 0x00}
	)
	b.Run("ReadArray", func(b *testing.B) {
		benchmarkReads(b, len(arrayWire), func() (n int, err error) {
			benchmarkHeaderSink, n, err = ReadArrayHeader(arrayWire)
			return n, err
		})
	})
	b.Run("ReadMap", func(b *testing.B) {
		benchmarkReads(b, len(mapWire), func() (n int, err error) {
			benchmarkHeaderSink, n, err = ReadMapHeader(mapWire)
			return n, err
		})
	})
}

func BenchmarkPayloads(b *testing.B) {
	tests := []struct {
		name string
		size int
	}{
		{name: "Small8", size: 8},
		{name: "Large65536", size: 65536},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			var (
				textValue    = strings.Repeat("x", tt.size)
				byteValue    = bytes.Repeat([]byte{0x78}, tt.size)
				stringOutput bytes.Buffer
				binaryOutput bytes.Buffer
				setupErr     error
			)
			setupErr = NewWriter(&stringOutput).EncodeString(textValue)
			if setupErr != nil {
				b.Fatal(setupErr)
			}
			setupErr = NewWriter(&binaryOutput).EncodeBytes(byteValue)
			if setupErr != nil {
				b.Fatal(setupErr)
			}
			var (
				stringWire = stringOutput.Bytes()
				binaryWire = binaryOutput.Bytes()
			)
			b.Run("ReadStringOwned", func(b *testing.B) {
				benchmarkReads(b, len(stringWire), func() (n int, err error) {
					benchmarkStringSink, n, err = ReadString(stringWire)
					return n, err
				})
			})
			b.Run("ReadStringBorrowed", func(b *testing.B) {
				benchmarkReads(b, len(stringWire), func() (n int, err error) {
					benchmarkBytesSink, n, err = ReadStringBytes(stringWire)
					return n, err
				})
			})
			b.Run("ReadBytesBorrowed", func(b *testing.B) {
				benchmarkReads(b, len(binaryWire), func() (n int, err error) {
					benchmarkBytesSink, n, err = ReadBytes(binaryWire)
					return n, err
				})
			})
		})
	}
}

func BenchmarkApplicationMessage(b *testing.B) {
	var (
		wire = applicationMessageFixture()
	)
	b.Run("Read", func(b *testing.B) {
		benchmarkReads(b, len(wire), func() (n int, err error) {
			benchmarkMessageSink, n, err = readApplicationMessage(wire)
			return n, err
		})
	})
}

func benchmarkReads(b *testing.B, size int, readValue func() (n int, err error)) {
	b.Helper()
	var err error
	b.ReportAllocs()
	b.SetBytes(int64(size))
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		benchmarkCountSink, err = readValue()
		if err != nil {
			b.Fatal(err)
		}
	}
}
