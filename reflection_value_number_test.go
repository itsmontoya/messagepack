package messagepack

import (
	"bytes"
	"testing"
)

type reflectionValueNumber int64

func (v reflectionValueNumber) DecodeMessagePack(r Reader) (err error) {
	_, err = r.DecodeBool()
	return err
}

func TestReflectionValueNumberHook(t *testing.T) {
	var (
		value = struct {
			Array [2]reflectionValueNumber
			Slice []reflectionValueNumber
		}{Array: [2]reflectionValueNumber{8, 9}}
		wire = []byte{0x82, 0xa5, 'S', 'l', 'i', 'c', 'e', 0x92, 0xc3, 0xc2,
			0xa5, 'A', 'r', 'r', 'a', 'y', 0x92, 0xc2, 0xc3}
		err = NewDecoder(bytes.NewReader(wire)).Decode(&value)
	)

	if err != nil || value.Array != [2]reflectionValueNumber{8, 9} || len(value.Slice) != 2 || value.Slice[0] != 0 || value.Slice[1] != 0 {
		t.Fatalf("Decode value hooks = (%+v, %v)", value, err)
	}
}
