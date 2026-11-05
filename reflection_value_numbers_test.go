package messagepack

import (
	"bytes"
	"testing"
)

type reflectionValueNumbers []int64

func (v reflectionValueNumbers) DecodeMessagePack(r Reader) (err error) {
	_, err = r.DecodeBool()
	return err
}

func TestReflectionValueNumbersHook(t *testing.T) {
	var (
		value = struct{ Values reflectionValueNumbers }{Values: reflectionValueNumbers{8}}
		wire  = []byte{0x81, 0xa6, 'V', 'a', 'l', 'u', 'e', 's', 0xc3}
		err   = NewDecoder(bytes.NewReader(wire)).Decode(&value)
	)

	if err != nil || len(value.Values) != 1 || value.Values[0] != 8 {
		t.Fatalf("Decode collection value hook = (%+v, %v)", value, err)
	}
}
