package messagepack

import (
	"bytes"
	"testing"
)

func FuzzDecoderReflection(f *testing.F) {
	for _, wire := range [][]byte{
		{}, {0xc0}, {0xc3}, {42}, {0xff}, {0xcd, 1}, {0xc1}, {0xd4},
		{0xc4, 0}, {0xc4, 2, 1, 2},
		{0x91, 42}, {0x81, 0xa2, 'I', 'D', 42}, {0x81, 0x90, 0xc0},
		{0xc6, 0xff, 0xff, 0xff, 0xff}, {0xdd, 0xff, 0xff, 0xff, 0xff},
	} {
		for kind := uint8(0); kind < 4; kind++ {
			f.Add(wire, kind)
		}
	}
	f.Fuzz(func(t *testing.T, wire []byte, kind uint8) {
		if len(wire) > 4096 {
			t.Skip()
		}

		var (
			input   = bytes.NewReader(wire)
			d       = NewDecoder(input)
			value   any
			array   []int16
			payload []byte
			backing *byte
			record  struct{ ID int }
			target  any = &value
			err     error
		)

		switch kind % 4 {
		case 1:
			target = &array
		case 2:
			target = &record
		case 3:
			payload = make([]byte, 8, 64)
			backing = &payload[0]
			target = &payload
		}
		if err = d.reader.SetLimits(ReaderLimits{MaxPayloadBytes: 4096}); err != nil {
			t.Fatal(err)
		}

		var (
			first     = d.Decode(target)
			remaining = input.Len()
		)

		if first != nil {
			if err = d.Decode(target); err != first || input.Len() != remaining {
				t.Fatalf("terminal Decode = %v, remaining %d, want (%v, %d)", err, input.Len(), first, remaining)
			}

			return
		}

		if kind%4 == 3 && payload != nil && len(payload) <= 64 {
			if cap(payload) != 64 || &payload[:cap(payload)][0] != backing {
				t.Fatal("Decode replaced sufficient byte-slice capacity")
			}
		}

		var (
			baseline = bytes.NewReader(wire)
			r        = NewReader(baseline)
		)

		if err = r.SetLimits(ReaderLimits{MaxSkipDepth: maximumSkipDepth}); err != nil {
			t.Fatal(err)
		}

		if err = r.Skip(); err != nil || baseline.Len() != remaining {
			t.Fatalf("successful Decode left %d bytes; Skip left (%d, %v)", remaining, baseline.Len(), err)
		}
	})
}
