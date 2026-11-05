package messagepack

// cpuBenchmarkMessage keeps the mixed schema without applicationMessage's hooks.
type cpuBenchmarkMessage applicationMessage

// encode uses the same cached wire fragments and typed Writer as reflection.
func (m *cpuBenchmarkMessage) encode(w *Writer, fields []reflectionField) (err error) {
	if err = w.writeEncoded(fields[0].prefix); err != nil {
		return err
	}

	if err = w.EncodeUint64(m.ID); err != nil {
		return err
	}

	if err = w.writeEncoded(fields[1].prefix); err != nil {
		return err
	}

	if err = w.EncodeString(m.Name); err != nil {
		return err
	}

	if err = w.writeEncoded(fields[2].prefix); err != nil {
		return err
	}

	if err = w.EncodeBool(m.Active); err != nil {
		return err
	}

	if err = w.writeEncoded(fields[3].prefix); err != nil {
		return err
	}

	if err = w.EncodeFloat64(m.Score); err != nil {
		return err
	}

	if err = w.writeEncoded(fields[4].prefix); err != nil {
		return err
	}

	if err = w.EncodeBytes(m.Payload); err != nil {
		return err
	}

	if err = w.writeEncoded(fields[5].prefix); err != nil {
		return err
	}

	if err = w.EncodeArrayHeader(2); err != nil {
		return err
	}

	for _, sample := range m.Samples {
		if err = w.EncodeInt64(sample); err != nil {
			return err
		}
	}
	return nil
}

// decode keeps name lookup and binary reuse, removing generic value traversal.
// This valid-message control is intentionally not a replacement public decoder.
func (m *cpuBenchmarkMessage) decode(r *Reader, fieldsByName map[string]int) (err error) {
	var count uint32
	if count, err = r.DecodeMapHeader(); err != nil {
		return err
	}

	for index := uint32(0); index < count; index++ {
		var (
			name       string
			fieldIndex int
			ok         bool
		)

		if name, err = r.DecodeString(); err != nil {
			return err
		}

		if fieldIndex, ok = fieldsByName[name]; !ok {
			if err = r.Skip(); err != nil {
				return err
			}

			continue
		}

		switch fieldIndex {
		case 0:
			m.ID, err = r.DecodeUint64()
		case 1:
			m.Name, err = r.DecodeString()
		case 2:
			m.Active, err = r.DecodeBool()
		case 3:
			m.Score, err = r.DecodeFloat64()
		case 4:
			m.Payload, err = r.readPayload("decode reflected bytes", false, m.Payload)
		case 5:
			err = m.decodeSamples(r)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *cpuBenchmarkMessage) decodeSamples(r *Reader) (err error) {
	var count uint32
	if count, err = r.DecodeArrayHeader(); err != nil {
		return err
	}

	if count != 2 {
		return ErrTypeMismatch
	}

	for index := range m.Samples {
		if m.Samples[index], err = r.DecodeInt64(); err != nil {
			return err
		}
	}
	return nil
}
