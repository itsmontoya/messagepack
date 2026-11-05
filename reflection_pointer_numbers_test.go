package messagepack

import (
	"bytes"
	"testing"
)

type reflectionPointerNumbers []float64

func (v *reflectionPointerNumbers) DecodeMessagePack(r Reader) (err error) {
	if err = r.DecodeNil(); err != nil {
		return err
	}

	*v = reflectionPointerNumbers{1.5}
	return nil
}

func TestReflectionPointerNumbersHook(t *testing.T) {
	var (
		value = struct{ Values reflectionPointerNumbers }{}
		wire  = []byte{0x81, 0xa6, 'V', 'a', 'l', 'u', 'e', 's', 0xc0}
		err   = NewDecoder(bytes.NewReader(wire)).Decode(&value)
	)

	if err != nil || len(value.Values) != 1 || value.Values[0] != 1.5 {
		t.Fatalf("Decode collection pointer hook = (%+v, %v)", value, err)
	}
}
