package messagepack

import "reflect"

// reflectedDecoder materializes one value within finite traversal and payload limits.
type reflectedDecoder struct {
	reader          Reader
	remainingVisits uint64
	started         bool
}

func (d *reflectedDecoder) decode(value reflect.Value, depth int) (err error) {
	if err = d.visit(depth); err != nil {
		return err
	}

	var plan = loadReflectionPlan(value.Type())
	if plan.kind == reflect.Pointer && value.IsNil() && value.CanSet() {
		value.Set(reflect.New(value.Type().Elem()))
	}

	if (plan.decodeHook || plan.kind == reflect.Interface) && value.CanInterface() {
		if hook, ok := value.Interface().(Decodee); ok {
			return hook.DecodeMessagePack(d.reader)
		}
	}

	if plan.decodeAddressHook && value.CanAddr() && value.Addr().CanInterface() {
		if hook, ok := value.Addr().Interface().(Decodee); ok {
			return hook.DecodeMessagePack(d.reader)
		}
	}

	var tag byte
	if tag, err = d.reader.peekTag(); err != nil {
		return err
	}

	d.started = true
	if tag == 0xc0 && plan.nullable {
		if err = d.reader.DecodeNil(); err != nil {
			return err
		}

		value.SetZero()
		return nil
	}

	return plan.decode(d, value, tag, depth)
}

func (d *reflectedDecoder) visit(depth int) (err error) {
	if depth == maximumReflectionDepth || d.remainingVisits == 0 {
		return d.reader.fail(ErrLimitExceeded)
	}

	d.remainingVisits--
	return nil
}

func (d *reflectedDecoder) decodePointer(value reflect.Value, _ byte, depth int) (err error) {
	return d.decode(value.Elem(), depth+1)
}

func (d *reflectedDecoder) decodeBool(value reflect.Value, _ byte, _ int) (err error) {
	var decoded bool
	if decoded, err = d.reader.DecodeBool(); err != nil {
		return err
	}

	value.SetBool(decoded)
	return nil
}

func (d *reflectedDecoder) decodeString(value reflect.Value, _ byte, _ int) (err error) {
	var decoded string
	if decoded, err = d.reader.DecodeString(); err != nil {
		return err
	}

	value.SetString(decoded)
	return nil
}

func (d *reflectedDecoder) decodeUnsupported(_ reflect.Value, _ byte, _ int) (err error) {
	return d.reader.fail(ErrUnsupportedType)
}

func (d *reflectedDecoder) decodeInt(value reflect.Value, _ byte, _ int) (err error) {
	var decoded int64
	if decoded, err = d.reader.DecodeInt64(); err != nil {
		return err
	}

	if value.OverflowInt(decoded) {
		return d.reader.fail(ErrOverflow)
	}

	value.SetInt(decoded)
	return nil
}

func (d *reflectedDecoder) decodeUint(value reflect.Value, _ byte, _ int) (err error) {
	var decoded uint64
	if decoded, err = d.reader.DecodeUint64(); err != nil {
		return err
	}

	if value.OverflowUint(decoded) {
		return d.reader.fail(ErrOverflow)
	}

	value.SetUint(decoded)
	return nil
}

func (d *reflectedDecoder) decodeFloat(value reflect.Value, _ byte, _ int) (err error) {
	if value.Kind() == reflect.Float32 {
		var narrow float32
		if narrow, err = d.reader.DecodeFloat32(); err != nil {
			return err
		}

		value.Set(reflect.ValueOf(narrow).Convert(value.Type()))
		return nil
	}

	var decoded float64
	if decoded, err = d.reader.DecodeFloat64(); err != nil {
		return err
	}

	value.SetFloat(decoded)
	return nil
}

func (d *reflectedDecoder) decodeBytes(value reflect.Value, _ byte, _ int) (err error) {
	var decoded []byte
	if decoded, err = d.reader.DecodeBytes(); err != nil {
		return err
	}

	value.SetBytes(decoded)
	return nil
}

func (d *reflectedDecoder) decodeArray(value reflect.Value, _ byte, depth int) (err error) {
	var count uint32
	if count, err = d.reader.DecodeArrayHeader(); err != nil {
		return err
	}

	if uint64(count) > d.remainingVisits {
		return d.reader.fail(ErrLimitExceeded)
	}

	if value.Kind() == reflect.Array {
		if uint64(count) != uint64(value.Len()) {
			return d.reader.fail(ErrTypeMismatch)
		}

		for index := 0; index < value.Len(); index++ {
			if err = d.decode(value.Index(index), depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	// Grow only after children arrive; a declared count alone never sizes storage.
	var decoded = reflect.MakeSlice(value.Type(), 0, 0)
	for index := uint32(0); index < count; index++ {
		var element = reflect.New(value.Type().Elem()).Elem()
		if err = d.decode(element, depth+1); err != nil {
			return err
		}

		decoded = reflect.Append(decoded, element)
	}
	value.Set(decoded)
	return nil
}

func (d *reflectedDecoder) decodeMap(value reflect.Value, _ byte, depth int) (err error) {
	var count uint32
	if count, err = d.reader.DecodeMapHeader(); err != nil {
		return err
	}

	if uint64(count)*2 > d.remainingVisits {
		return d.reader.fail(ErrLimitExceeded)
	}

	var decoded = reflect.MakeMap(value.Type())
	for index := uint32(0); index < count; index++ {
		var (
			key     = reflect.New(value.Type().Key()).Elem()
			element = reflect.New(value.Type().Elem()).Elem()
		)

		if err = d.decode(key, depth+1); err != nil {
			return err
		}

		if !key.Comparable() {
			return d.reader.fail(ErrTypeMismatch)
		}

		if err = d.decode(element, depth+1); err != nil {
			return err
		}

		decoded.SetMapIndex(key, element)
	}
	value.Set(decoded)
	return nil
}

func (d *reflectedDecoder) decodeStruct(value reflect.Value, fields map[string]int, depth int) (err error) {
	var count uint32
	if count, err = d.reader.DecodeMapHeader(); err != nil {
		return err
	}

	if uint64(count)*2 > d.remainingVisits {
		return d.reader.fail(ErrLimitExceeded)
	}

	for index := uint32(0); index < count; index++ {
		if err = d.visit(depth + 1); err != nil {
			return err
		}

		var name string
		if name, err = d.reader.DecodeString(); err != nil {
			return err
		}

		if field, ok := fields[name]; ok {
			if err = d.decode(value.Field(field), depth+1); err != nil {
				return err
			}

			continue
		}

		if d.remainingVisits == 0 {
			return d.reader.fail(ErrLimitExceeded)
		}

		d.remainingVisits--
		if err = d.reader.Skip(); err != nil {
			return err
		}
	}
	return nil
}

func (d *reflectedDecoder) decodeInterface(value reflect.Value, tag byte, depth int) (err error) {
	if !value.IsNil() && value.Elem().Kind() == reflect.Pointer && !value.Elem().IsNil() {
		return d.decode(value.Elem(), depth+1)
	}

	if value.NumMethod() != 0 {
		return d.reader.fail(ErrUnsupportedType)
	}

	var typ reflect.Type
	switch {
	case tag <= 0x7f || tag >= 0xcc && tag <= 0xcf:
		typ = reflect.TypeFor[uint64]()
	case tag >= 0xe0 || tag >= 0xd0 && tag <= 0xd3:
		typ = reflect.TypeFor[int64]()
	case tag == 0xc2 || tag == 0xc3:
		typ = reflect.TypeFor[bool]()
	case tag == 0xca:
		typ = reflect.TypeFor[float32]()
	case tag == 0xcb:
		typ = reflect.TypeFor[float64]()
	case tag >= 0xa0 && tag <= 0xbf || tag >= 0xd9 && tag <= 0xdb:
		typ = reflect.TypeFor[string]()
	case tag >= 0xc4 && tag <= 0xc6:
		typ = reflect.TypeFor[[]byte]()
	case tag >= 0x90 && tag <= 0x9f || tag == 0xdc || tag == 0xdd:
		typ = reflect.TypeFor[[]any]()
	case tag >= 0x80 && tag <= 0x8f || tag == 0xde || tag == 0xdf:
		typ = reflect.TypeFor[map[any]any]()
	default:
		return d.reader.fail(readTypeError(tag))
	}
	var decoded = reflect.New(typ).Elem()
	if err = d.decode(decoded, depth+1); err != nil {
		return err
	}

	value.Set(decoded)
	return nil
}
