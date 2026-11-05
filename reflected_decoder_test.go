package messagepack

import (
	"bytes"
	"reflect"
	"testing"
)

func TestReflectedDecoderNumericTraversal(t *testing.T) {
	tests := []struct {
		name      string
		wire      []byte
		depth     int
		visits    uint64
		remaining uint64
		consumed  int
		wantErr   error
	}{
		{name: "exact budget", wire: []byte{0x92, 7, 8}, visits: 3, consumed: 3},
		{name: "count budget", wire: []byte{0x92, 7, 8}, visits: 2, remaining: 1, consumed: 1, wantErr: ErrLimitExceeded},
		{name: "budget before length", wire: []byte{0x93, 7, 8, 9}, visits: 3, remaining: 2, consumed: 1, wantErr: ErrLimitExceeded},
		{name: "depth before children", wire: []byte{0x92, 7, 8}, depth: maximumReflectionDepth - 1, visits: 3, remaining: 2, consumed: 1, wantErr: ErrLimitExceeded},
		{name: "root depth", wire: []byte{0x92, 7, 8}, depth: maximumReflectionDepth, visits: 3, remaining: 3, wantErr: ErrLimitExceeded},
		{name: "last scalar depth", wire: []byte{0x92, 7, 8}, depth: maximumReflectionDepth - 2, visits: 3, consumed: 3},
		{name: "failed child visit", wire: []byte{0x92, 7, 0xc3}, visits: 4, remaining: 1, consumed: 3, wantErr: ErrTypeMismatch},
		{name: "fixed length", wire: []byte{0x93, 0xc3}, visits: 8, remaining: 7, consumed: 1, wantErr: ErrTypeMismatch},
	}

	for _, tt := range tests {
		for _, typ := range []reflect.Type{reflect.TypeFor[[2]int64](), reflect.TypeFor[[2]uint64]()} {
			t.Run(tt.name+"/"+typ.String(), func(t *testing.T) {
				var (
					source  = bytes.NewReader(tt.wire)
					decoder = reflectedDecoder{reader: *NewReader(source), remainingVisits: tt.visits}
					target  = reflect.New(typ).Elem()
					err     = decoder.decode(target, tt.depth)
				)

				if err != tt.wantErr || decoder.remainingVisits != tt.remaining || len(tt.wire)-source.Len() != tt.consumed {
					t.Fatalf("decode = %v, remaining %d, consumed %d; want %v, remaining %d, consumed %d", err, decoder.remainingVisits, len(tt.wire)-source.Len(), tt.wantErr, tt.remaining, tt.consumed)
				}
			})
		}
	}
}

func TestReflectedDecoderNumericEmptyAtDepthLimit(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeFor[[]int64](), reflect.TypeFor[[]uint64](), reflect.TypeFor[[]float64](), reflect.TypeFor[[0]int64]()} {
		var (
			source  = bytes.NewReader([]byte{0x90, 0xc3})
			decoder = reflectedDecoder{reader: *NewReader(source), remainingVisits: 1}
			target  = reflect.New(typ).Elem()
			err     = decoder.decode(target, maximumReflectionDepth-1)
		)

		if err != nil || source.Len() != 1 || decoder.remainingVisits != 0 {
			t.Fatalf("decode %s = %v, remaining bytes %d, visits %d", typ, err, source.Len(), decoder.remainingVisits)
		}
	}
}
