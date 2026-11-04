package messagepack

import (
	"errors"
	"fmt"
	"io"
)

// encodeStreamingApplicationMessage writes the same strict schema as the slice example.
func encodeStreamingApplicationMessage(e *Writer, message applicationMessage) (err error) {
	if err = e.EncodeMapHeader(6); err != nil {
		return err
	}

	for _, key := range []string{"id", "name", "active", "score", "data", "samples"} {
		if err = e.EncodeString(key); err != nil {
			return err
		}

		switch key {
		case "id":
			err = e.EncodeUint64(message.ID)
		case "name":
			err = e.EncodeString(message.Name)
		case "active":
			err = e.EncodeBool(message.Active)
		case "score":
			err = e.EncodeFloat64(message.Score)
		case "data":
			err = e.EncodeBytes(message.Payload)
		case "samples":
			err = encodeStreamingApplicationSamples(e, message.Samples)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func encodeStreamingApplicationSamples(e *Writer, samples [2]int64) (err error) {
	if err = e.EncodeArrayHeader(2); err != nil {
		return err
	}

	for _, sample := range samples {
		if err = e.EncodeInt64(sample); err != nil {
			return err
		}
	}
	return nil
}

// decodeStreamingApplicationMessage owns its string/binary results and leaves the next root.
// The schema requires six ordered fields; EOF after its root is incomplete input.
func decodeStreamingApplicationMessage(d *Reader) (message applicationMessage, err error) {
	var (
		count uint32
		key   string
	)
	if count, err = d.DecodeMapHeader(); err != nil {
		return applicationMessage{}, err
	}

	if count != 6 {
		return applicationMessage{}, errApplicationSchema
	}

	for _, expected := range []string{"id", "name", "active", "score", "data", "samples"} {
		if key, err = d.DecodeString(); err != nil {
			return applicationMessage{}, streamingApplicationChildError(err)
		}

		if key != expected {
			return applicationMessage{}, errApplicationSchema
		}

		switch key {
		case "id":
			message.ID, err = d.DecodeUint64()
		case "name":
			message.Name, err = d.DecodeString()
		case "active":
			message.Active, err = d.DecodeBool()
		case "score":
			message.Score, err = d.DecodeFloat64()
		case "data":
			message.Payload, err = d.DecodeBytes()
		case "samples":
			message.Samples, err = decodeStreamingApplicationSamples(d)
		}
		if err != nil {
			return applicationMessage{}, streamingApplicationChildError(err)
		}
	}
	return message, nil
}

func decodeStreamingApplicationSamples(d *Reader) (samples [2]int64, err error) {
	var count uint32
	if count, err = d.DecodeArrayHeader(); err != nil {
		return [2]int64{}, err
	}

	if count != 2 {
		return [2]int64{}, errApplicationSchema
	}

	for index := range samples {
		if samples[index], err = d.DecodeInt64(); err != nil {
			return [2]int64{}, err
		}
	}
	return samples, nil
}

func streamingApplicationChildError(err error) (out error) {
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("example: incomplete application message: %w", io.ErrUnexpectedEOF)
	}

	return err
}
