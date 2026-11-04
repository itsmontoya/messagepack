package messagepack

import (
	"bytes"
	"errors"
)

var (
	errApplicationSchema   = errors.New("example: unexpected field or container count")
	errApplicationTrailing = errors.New("example: unwanted trailing bytes")
)

// applicationMessage is an example schema, not part of the codec's public API.
type applicationMessage struct {
	ID      uint64
	Name    string
	Active  bool
	Score   float64
	Payload []byte
	Samples [2]int64
}

// EncodeMessagePack writes the same explicit schema used by the streaming examples.
func (m applicationMessage) EncodeMessagePack(w Writer) (err error) {
	return encodeStreamingApplicationMessage(&w, m)
}

// DecodeMessagePack replaces the message only after its strict schema is complete.
func (m *applicationMessage) DecodeMessagePack(r Reader) (err error) {
	var value applicationMessage
	value, err = decodeStreamingApplicationMessage(&r)
	if err != nil {
		return err
	}
	*m = value
	return nil
}

// readApplicationMessage reads one schema value and reports consumed stream bytes.
func readApplicationMessage(src []byte) (message applicationMessage, n int, err error) {
	input := bytes.NewReader(src)
	err = NewDecoder(input).Decode(&message)
	return message, len(src) - input.Len(), err
}

func readCompleteApplicationMessage(src []byte) (message applicationMessage, err error) {
	var n int
	message, n, err = readApplicationMessage(src)
	if err != nil {
		return applicationMessage{}, err
	}
	if n != len(src) {
		return applicationMessage{}, errApplicationTrailing
	}
	return message, nil
}

func applicationMessageFixture() (wire []byte) {
	return []byte{
		0x86,
		0xa2, 0x69, 0x64, 0x07, // id: 7
		0xa4, 0x6e, 0x61, 0x6d, 0x65, 0xa3, 0x41, 0x64, 0x61, // name: Ada
		0xa6, 0x61, 0x63, 0x74, 0x69, 0x76, 0x65, 0xc3, // active: true
		0xa5, 0x73, 0x63, 0x6f, 0x72, 0x65,
		0xcb, 0x3f, 0xf8, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // score: 1.5
		0xa4, 0x64, 0x61, 0x74, 0x61, 0xc4, 0x02, 0x01, 0x02, // data: 01 02
		0xa7, 0x73, 0x61, 0x6d, 0x70, 0x6c, 0x65, 0x73, 0x92, 0xff, 0x02, // samples: [-1, 2]
	}
}
