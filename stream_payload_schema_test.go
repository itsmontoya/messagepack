package messagepack

import "io"

// encodeStreamingPayloadMessage streams data within the existing strict six-field schema.
func encodeStreamingPayloadMessage(e *Encoder, length uint64, src io.Reader) (err error) {
	if err = e.EncodeMapHeader(6); err != nil {
		return err
	}
	for _, key := range []string{"id", "name", "active", "score", "data", "samples"} {
		if err = e.EncodeString(key); err != nil {
			return err
		}
		switch key {
		case "id":
			err = e.EncodeUint64(7)
		case "name":
			err = e.EncodeString("Ada")
		case "active":
			err = e.EncodeBool(true)
		case "score":
			err = e.EncodeFloat64(1.5)
		case "data":
			_, err = e.EncodeBytesFrom(length, src)
		case "samples":
			err = encodeStreamingApplicationSamples(e, [2]int64{-1, 2})
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// decodeStreamingPayloadMessage retains small fields and sends data directly to dst.
// A failed message can already have delivered a payload prefix to dst.
func decodeStreamingPayloadMessage(d *Decoder, dst io.Writer) (message applicationMessage, n uint64, err error) {
	var (
		count uint32
		key   string
	)
	count, err = d.DecodeMapHeader()
	if err != nil {
		return applicationMessage{}, 0, err
	}
	if count != 6 {
		return applicationMessage{}, 0, errApplicationSchema
	}
	for _, expected := range []string{"id", "name", "active", "score", "data", "samples"} {
		key, err = d.DecodeString()
		if err != nil {
			return applicationMessage{}, n, streamingApplicationChildError(err)
		}
		if key != expected {
			return applicationMessage{}, n, errApplicationSchema
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
			n, err = d.DecodeBytesTo(dst)
		case "samples":
			message.Samples, err = decodeStreamingApplicationSamples(d)
		}
		if err != nil {
			return applicationMessage{}, n, streamingApplicationChildError(err)
		}
	}
	return message, n, nil
}
