package messagepack

import "reflect"

// reflectedDecoder materializes one value within finite traversal and payload limits.
type reflectedDecoder struct {
	reader          Reader
	remainingVisits uint64
	started         bool
}

func (d *reflectedDecoder) decode(value reflect.Value, depth int) (err error) {
	return d.decodePlanned(value, lookupReflectionPlan(value.Type()), depth)
}

func (d *reflectedDecoder) decodePlanned(value reflect.Value, plan *reflectionPlan, depth int) (err error) {
	if err = d.visit(depth); err != nil {
		return err
	}

	plan.once.Do(plan.initialize)
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

func (d *reflectedDecoder) decodePointer(value reflect.Value, element *reflectionPlan, depth int) (err error) {
	return d.decodePlanned(value.Elem(), element, depth+1)
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
	if decoded, err = d.reader.readPayload("decode reflected bytes", false, value.Bytes()); err != nil {
		return err
	}

	value.SetBytes(decoded)
	return nil
}

func (d *reflectedDecoder) decodeArray(value reflect.Value, element *reflectionPlan, decodeElements func(*reflectedDecoder, reflect.Value, uint32) error, depth int) (err error) {
	var count uint32
	if count, err = d.reader.DecodeArrayHeader(); err != nil {
		return err
	}

	if uint64(count) > d.remainingVisits {
		return d.reader.fail(ErrLimitExceeded)
	}

	if value.Kind() == reflect.Array && uint64(count) != uint64(value.Len()) {
		return d.reader.fail(ErrTypeMismatch)
	}

	if decodeElements != nil {
		if count != 0 && depth+1 == maximumReflectionDepth {
			return d.reader.fail(ErrLimitExceeded)
		}

		return decodeElements(d, value, count)
	}

	if value.Kind() == reflect.Array {
		for index := 0; index < value.Len(); index++ {
			if err = d.decodePlanned(value.Index(index), element, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	// Grow only after children arrive; a declared count alone never sizes storage.
	var decoded = reflect.MakeSlice(value.Type(), 0, 0)
	for index := uint32(0); index < count; index++ {
		var child = reflect.New(value.Type().Elem()).Elem()
		if err = d.decodePlanned(child, element, depth+1); err != nil {
			return err
		}

		decoded = reflect.Append(decoded, child)
	}
	value.Set(decoded)
	return nil
}

func (d *reflectedDecoder) decodeInt64s(value reflect.Value, count uint32) (err error) {
	var (
		scratch [reflectionBatchElements]int64
		decoded = []int64{}
	)

	for offset := uint32(0); offset < count; {
		var (
			requested = int(min(count-offset, reflectionBatchElements))
			completed int
		)

		completed, err = d.reader.decodeInt64s(scratch[:requested], value.Kind() == reflect.Slice)
		d.visitNumericBatch(completed, requested, err)
		if value.Kind() == reflect.Array {
			for index := 0; index < completed; index++ {
				value.Index(int(offset) + index).SetInt(scratch[index])
			}
		} else {
			decoded = append(decoded, scratch[:completed]...)
		}

		if err != nil {
			return err
		}

		offset += uint32(completed)
	}
	if value.Kind() == reflect.Slice {
		value.Set(reflect.ValueOf(decoded))
	}

	return nil
}

func (d *reflectedDecoder) decodeUint64s(value reflect.Value, count uint32) (err error) {
	var (
		scratch [reflectionBatchElements]uint64
		decoded = []uint64{}
	)

	for offset := uint32(0); offset < count; {
		var (
			requested = int(min(count-offset, reflectionBatchElements))
			completed int
		)

		completed, err = d.reader.decodeUint64s(scratch[:requested], value.Kind() == reflect.Slice)
		d.visitNumericBatch(completed, requested, err)
		if value.Kind() == reflect.Array {
			for index := 0; index < completed; index++ {
				value.Index(int(offset) + index).SetUint(scratch[index])
			}
		} else {
			decoded = append(decoded, scratch[:completed]...)
		}

		if err != nil {
			return err
		}

		offset += uint32(completed)
	}
	if value.Kind() == reflect.Slice {
		value.Set(reflect.ValueOf(decoded))
	}

	return nil
}

func (d *reflectedDecoder) decodeFloat64s(value reflect.Value, count uint32) (err error) {
	var (
		scratch [reflectionBatchElements]float64
		decoded = []float64{}
	)

	for offset := uint32(0); offset < count; {
		var (
			requested = int(min(count-offset, reflectionBatchElements))
			completed int
		)

		completed, err = d.reader.decodeFloat64s(scratch[:requested], value.Kind() == reflect.Slice)
		d.visitNumericBatch(completed, requested, err)
		if value.Kind() == reflect.Array {
			for index := 0; index < completed; index++ {
				value.Index(int(offset) + index).SetFloat(scratch[index])
			}
		} else {
			decoded = append(decoded, scratch[:completed]...)
		}

		if err != nil {
			return err
		}

		offset += uint32(completed)
	}
	if value.Kind() == reflect.Slice {
		value.Set(reflect.ValueOf(decoded))
	}

	return nil
}

// visitNumericBatch charges successful children and the attempted failing child.
// decodeArray has already checked the whole count and the children's depth.
func (d *reflectedDecoder) visitNumericBatch(completed, requested int, err error) {
	d.remainingVisits -= uint64(completed)
	if err != nil && completed < requested {
		d.remainingVisits--
	}
}

func (d *reflectedDecoder) decodeMap(value reflect.Value, key, element *reflectionPlan, depth int) (err error) {
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
			decodedKey   = reflect.New(value.Type().Key()).Elem()
			decodedValue = reflect.New(value.Type().Elem()).Elem()
		)

		if err = d.decodePlanned(decodedKey, key, depth+1); err != nil {
			return err
		}

		if !decodedKey.Comparable() {
			return d.reader.fail(ErrTypeMismatch)
		}

		if err = d.decodePlanned(decodedValue, element, depth+1); err != nil {
			return err
		}

		decoded.SetMapIndex(decodedKey, decodedValue)
	}
	value.Set(decoded)
	return nil
}

func (d *reflectedDecoder) decodeStruct(value reflect.Value, fields []reflectionField, fieldsByName map[string]int, depth int) (err error) {
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

		if fieldIndex, ok := fieldsByName[name]; ok {
			var field = fields[fieldIndex]
			if err = d.decodePlanned(value.Field(field.index), field.plan, depth+1); err != nil {
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
