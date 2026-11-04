package messagepack

// decodeFlexibleStreamingApplicationMessage accepts six required fields in any order.
// This example schema permits at most 16 entries, rejects every duplicate name,
// and skips unknown supported values. The codec itself retains uint32 map counts.
func decodeFlexibleStreamingApplicationMessage(d *Reader) (message applicationMessage, err error) {
	var (
		count    uint32
		seen     [16]string
		required uint8
		key      string
	)
	if count, err = d.DecodeMapHeader(); err != nil {
		return applicationMessage{}, err
	}

	if count < 6 || count > uint32(len(seen)) {
		return applicationMessage{}, errApplicationSchema
	}

	for index := uint32(0); index < count; index++ {
		if key, err = d.DecodeString(); err != nil {
			return applicationMessage{}, streamingApplicationChildError(err)
		}

		for earlier := uint32(0); earlier < index; earlier++ {
			if seen[earlier] == key {
				return applicationMessage{}, errApplicationSchema
			}
		}
		seen[index] = key
		switch key {
		case "id":
			message.ID, err = d.DecodeUint64()
			required |= 1
		case "name":
			message.Name, err = d.DecodeString()
			required |= 2
		case "active":
			message.Active, err = d.DecodeBool()
			required |= 4
		case "score":
			message.Score, err = d.DecodeFloat64()
			required |= 8
		case "data":
			message.Payload, err = d.DecodeBytes()
			required |= 16
		case "samples":
			message.Samples, err = decodeStreamingApplicationSamples(d)
			required |= 32
		default:
			err = d.Skip()
		}
		if err != nil {
			return applicationMessage{}, streamingApplicationChildError(err)
		}
	}
	if required != 63 {
		return applicationMessage{}, errApplicationSchema
	}

	return message, nil
}

func flexibleStreamingApplicationFixture() (wire []byte) {
	return []byte{
		0x87,
		0xa6, 'f', 'u', 't', 'u', 'r', 'e',
		0x81, 0xa1, 'v', 0x93, 0xc0, 0xc4, 0x02, 0xff, 0x00, 0x80,
		0xa7, 's', 'a', 'm', 'p', 'l', 'e', 's', 0x92, 0xff, 0x02,
		0xa4, 'd', 'a', 't', 'a', 0xc4, 0x02, 0x01, 0x02,
		0xa5, 's', 'c', 'o', 'r', 'e',
		0xcb, 0x3f, 0xf8, 0, 0, 0, 0, 0, 0,
		0xa6, 'a', 'c', 't', 'i', 'v', 'e', 0xc3,
		0xa4, 'n', 'a', 'm', 'e', 0xa3, 'A', 'd', 'a',
		0xa2, 'i', 'd', 0x07,
	}
}
