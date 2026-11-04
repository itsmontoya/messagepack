package messagepack_test

import (
	"fmt"

	"github.com/itsmontoya/messagepack"
)

type exampleMessage struct {
	ID uint64
}

// EncodeMessagePack writes exampleMessage's explicit one-field map schema.
func (m exampleMessage) EncodeMessagePack(w messagepack.Writer) (err error) {
	if err = w.EncodeMapHeader(1); err != nil {
		return err
	}
	if err = w.EncodeString("id"); err != nil {
		return err
	}
	return w.EncodeUint64(m.ID)
}

// DecodeMessagePack validates exampleMessage's explicit one-field map schema.
func (m *exampleMessage) DecodeMessagePack(r messagepack.Reader) (err error) {
	var (
		count uint32
		key   string
	)
	count, err = r.DecodeMapHeader()
	if err != nil {
		return err
	}
	if count != 1 {
		return fmt.Errorf("example: expected one field, got %d", count)
	}
	key, err = r.DecodeString()
	if err != nil {
		return err
	}
	if key != "id" {
		return fmt.Errorf("example: unexpected key %q", key)
	}
	m.ID, err = r.DecodeUint64()
	return err
}
