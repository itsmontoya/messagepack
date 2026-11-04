package messagepack

import "errors"

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

// readApplicationMessage requires exactly these six fields in this order.
// Unknown, duplicate, or reordered fields fail; unknown-field skipping is deferred.
// Payload borrows src, while Name is owned. Success may leave another message.
func readApplicationMessage(src []byte) (message applicationMessage, n int, err error) {
	var (
		count uint32
		step  int
	)
	count, n, err = ReadMapHeader(src)
	if err != nil {
		return applicationMessage{}, 0, err
	}
	if count != 6 {
		return applicationMessage{}, 0, errApplicationSchema
	}
	for _, key := range []string{"id", "name", "active", "score", "data", "samples"} {
		step, err = readApplicationKey(src[n:], key)
		if err != nil {
			return applicationMessage{}, 0, err
		}
		n += step
		switch key {
		case "id":
			message.ID, step, err = ReadUint64(src[n:])
		case "name":
			message.Name, step, err = ReadString(src[n:])
		case "active":
			message.Active, step, err = ReadBool(src[n:])
		case "score":
			message.Score, step, err = ReadFloat64(src[n:])
		case "data":
			message.Payload, step, err = ReadBytes(src[n:])
		case "samples":
			message.Samples, step, err = readApplicationSamples(src[n:])
		}
		if err != nil {
			return applicationMessage{}, 0, err
		}
		n += step
	}
	return message, n, nil
}

func readApplicationKey(src []byte, expected string) (n int, err error) {
	var key []byte
	key, n, err = ReadStringBytes(src)
	if err != nil {
		return 0, err
	}
	if string(key) != expected {
		return 0, errApplicationSchema
	}
	return n, nil
}

func readApplicationSamples(src []byte) (samples [2]int64, n int, err error) {
	var (
		count uint32
		step  int
	)
	count, n, err = ReadArrayHeader(src)
	if err != nil {
		return [2]int64{}, 0, err
	}
	if count != 2 {
		return [2]int64{}, 0, errApplicationSchema
	}
	for index := range samples {
		samples[index], step, err = ReadInt64(src[n:])
		if err != nil {
			return [2]int64{}, 0, err
		}
		n += step
	}
	return samples, n, nil
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
