package messagepack

import (
	"bytes"
	"math"
	"testing"
)

func FuzzEncoderReflection(f *testing.F) {
	f.Add(uint64(7), int64(-1), uint64(0x3ff8000000000000), "Ada", []byte{1, 2}, true)
	f.Add(uint64(math.MaxUint64), int64(math.MinInt64), uint64(0x7ff0000000000001), "\x00\xff", []byte{}, false)
	f.Fuzz(func(t *testing.T, id uint64, count int64, scoreBits uint64, name string, payload []byte, active bool) {
		if len(name)+len(payload) > 4096 {
			t.Skip()
		}

		var (
			value = struct {
				ID      uint64
				Count   int64
				Score   float64
				Name    string
				Payload []byte
				Active  bool
			}{ID: id, Count: count, Score: math.Float64frombits(scoreBits), Name: name, Payload: payload, Active: active}
			output bytes.Buffer
			e      = NewEncoder(&output)
			err    error
		)

		for index := 0; index < 2; index++ {
			if err = e.Encode(&value); err != nil {
				t.Fatal(err)
			}

			value.ID ^= 1
		}
		var (
			d      = NewDecoder(&output)
			target struct {
				ID      uint64
				Count   int64
				Score   float64
				Name    string
				Payload []byte
				Active  bool
			}
		)

		for index := 0; index < 2; index++ {
			if err = d.Decode(&target); err != nil {
				t.Fatal(err)
			}

			if target.ID != value.ID || target.Count != count || math.Float64bits(target.Score) != scoreBits ||
				target.Name != name || !bytes.Equal(target.Payload, payload) || target.Active != active {
				t.Fatalf("decoded struct differs from source: %+v", target)
			}

			value.ID ^= 1
		}
		if output.Len() != 0 {
			t.Fatalf("decoder left %d encoded bytes", output.Len())
		}
	})
}
