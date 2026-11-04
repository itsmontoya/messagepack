package messagepack_test

import "github.com/itsmontoya/messagepack"

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
