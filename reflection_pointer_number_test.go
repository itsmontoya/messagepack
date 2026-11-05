package messagepack

import (
	"bytes"
	"reflect"
	"testing"
)

type reflectionPointerNumber uint64

func (v *reflectionPointerNumber) DecodeMessagePack(r Reader) (err error) {
	var enabled bool
	if enabled, err = r.DecodeBool(); err != nil {
		return err
	}

	*v = 8
	if enabled {
		*v = 9
	}

	return nil
}

func TestReflectionPointerNumberHook(t *testing.T) {
	var (
		value = struct {
			Array [2]reflectionPointerNumber
			Slice []reflectionPointerNumber
		}{}
		wire = []byte{0x82, 0xa5, 'S', 'l', 'i', 'c', 'e', 0x92, 0xc3, 0xc2,
			0xa5, 'A', 'r', 'r', 'a', 'y', 0x92, 0xc2, 0xc3}
		err = NewDecoder(bytes.NewReader(wire)).Decode(&value)
	)

	if err != nil || value.Array != [2]reflectionPointerNumber{8, 9} || !reflect.DeepEqual(value.Slice, []reflectionPointerNumber{9, 8}) {
		t.Fatalf("Decode pointer hooks = (%+v, %v)", value, err)
	}
}
