package messagepack

import "testing"

func TestZeroAllocations(t *testing.T) {
	var (
		dst        = make([]byte, 0, 128)
		stringWire = []byte{0xa8, 'm', 's', 'g', 'p', 'a', 'c', 'k', '!'}
		binaryWire = []byte{0xc4, 0x08, 'm', 's', 'g', 'p', 'a', 'c', 'k', '!'}
		tests      = []struct {
			name string
			run  func()
		}{
			{name: "primitive reads", run: func() {
				var err error
				benchmarkCountSink, err = ReadNil([]byte{0xc0})
				if err != nil {
					t.Fatal(err)
				}
				benchmarkBoolSink, benchmarkCountSink, err = ReadBool([]byte{0xc3})
				if err != nil {
					t.Fatal(err)
				}
				benchmarkIntSink, benchmarkCountSink, err = ReadInt64([]byte{0xd2, 0xff, 0xff, 0x7f, 0xff})
				if err != nil {
					t.Fatal(err)
				}
				benchmarkUintSink, benchmarkCountSink, err = ReadUint64([]byte{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
				if err != nil {
					t.Fatal(err)
				}
				benchmarkFloat32Sink, benchmarkCountSink, err = ReadFloat32([]byte{0xca, 0x3f, 0xc0, 0x00, 0x00})
				if err != nil {
					t.Fatal(err)
				}
				benchmarkFloat64Sink, benchmarkCountSink, err = ReadFloat64([]byte{0xcb, 0x3f, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00})
				if err != nil {
					t.Fatal(err)
				}
			}},
			{name: "borrowed payload reads", run: func() {
				var err error
				benchmarkBytesSink, benchmarkCountSink, err = ReadStringBytes(stringWire)
				if err != nil {
					t.Fatal(err)
				}
				benchmarkBytesSink, benchmarkCountSink, err = ReadBytes(binaryWire)
				if err != nil {
					t.Fatal(err)
				}
			}},
			{name: "header reads", run: func() {
				var err error
				benchmarkHeaderSink, benchmarkCountSink, err = ReadArrayHeader([]byte{0xdd, 0x00, 0x01, 0x00, 0x00})
				if err != nil {
					t.Fatal(err)
				}
				benchmarkHeaderSink, benchmarkCountSink, err = ReadMapHeader([]byte{0xdf, 0x00, 0x01, 0x00, 0x00})
				if err != nil {
					t.Fatal(err)
				}
			}},
			{name: "preallocated appends", run: func() {
				var (
					out = AppendNil(dst[:0])
					err error
				)
				out = AppendBool(out, true)
				out = AppendInt64(out, -32769)
				out = AppendUint64(out, ^uint64(0))
				out = AppendFloat32(out, 1.5)
				out = AppendFloat64(out, 1.5)
				out = AppendArrayHeader(out, 65536)
				out = AppendMapHeader(out, 65536)
				out, err = AppendString(out, "msgpack!")
				if err != nil {
					t.Fatal(err)
				}
				out, err = AppendBytes(out, binaryWire[2:])
				if err != nil {
					t.Fatal(err)
				}
				benchmarkBytesSink = out
			}},
		}
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allocations := testing.AllocsPerRun(1000, tt.run)
			if allocations != 0 {
				t.Fatalf("measured %g allocations per run, want zero", allocations)
			}
		})
	}
}

func TestOwnedStringAllocations(t *testing.T) {
	wire := []byte{0xa8, 'm', 's', 'g', 'p', 'a', 'c', 'k', '!'}
	allocations := testing.AllocsPerRun(1000, func() {
		var err error
		benchmarkStringSink, benchmarkCountSink, err = ReadString(wire)
		if err != nil {
			t.Fatal(err)
		}
	})
	t.Logf("owned 8-byte string escaping to a test sink: %g allocations per read", allocations)
}
