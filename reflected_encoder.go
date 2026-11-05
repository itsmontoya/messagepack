package messagepack

import (
	"math"
	"reflect"
)

// reflectedEncoder traverses one value while sharing the Encoder's Writer state.
type reflectedEncoder struct {
	writer Writer
}

func (e *reflectedEncoder) encode(value reflect.Value, depth int) (err error) {
	if depth == maximumReflectionDepth {
		return e.fail(ErrLimitExceeded)
	}

	if !value.IsValid() {
		return e.writer.EncodeNil()
	}

	return e.encodePlanned(value, lookupReflectionPlan(value.Type()), depth)
}

func (e *reflectedEncoder) encodePlanned(value reflect.Value, plan *reflectionPlan, depth int) (err error) {
	if depth == maximumReflectionDepth {
		return e.fail(ErrLimitExceeded)
	}

	plan.once.Do(plan.initialize)
	if (plan.encodeHook || plan.kind == reflect.Interface) && value.CanInterface() {
		if hook, ok := value.Interface().(Encodee); ok {
			return hook.EncodeMessagePack(e.writer)
		}
	}

	if plan.encodeAddressHook && value.CanAddr() && value.Addr().CanInterface() {
		if hook, ok := value.Addr().Interface().(Encodee); ok {
			return hook.EncodeMessagePack(e.writer)
		}
	}

	return plan.encode(e, value, depth)
}

func (e *reflectedEncoder) encodeIndirect(value reflect.Value, depth int) (err error) {
	if value.IsNil() {
		return e.writer.EncodeNil()
	}

	return e.encode(value.Elem(), depth+1)
}

func (e *reflectedEncoder) encodePointer(value reflect.Value, element *reflectionPlan, depth int) (err error) {
	if value.IsNil() {
		return e.writer.EncodeNil()
	}

	return e.encodePlanned(value.Elem(), element, depth+1)
}

func (e *reflectedEncoder) encodeBool(value reflect.Value, _ int) (err error) {
	return e.writer.EncodeBool(value.Bool())
}

func (e *reflectedEncoder) encodeInt(value reflect.Value, _ int) (err error) {
	return e.writer.EncodeInt64(value.Int())
}

func (e *reflectedEncoder) encodeUint(value reflect.Value, _ int) (err error) {
	return e.writer.EncodeUint64(value.Uint())
}

func (e *reflectedEncoder) encodeFloat32(value reflect.Value, _ int) (err error) {
	if value.CanAddr() {
		// A typed pointer preserves NaN bits without boxing the field value.
		return e.writer.EncodeFloat32(*value.Addr().Convert(reflect.TypeFor[*float32]()).Interface().(*float32))
	}

	if value.Type() == reflect.TypeFor[float32]() {
		return e.writer.EncodeFloat32(value.Interface().(float32))
	}

	return e.writer.EncodeFloat32(value.Convert(reflect.TypeFor[float32]()).Interface().(float32))
}

func (e *reflectedEncoder) encodeFloat64(value reflect.Value, _ int) (err error) {
	return e.writer.EncodeFloat64(value.Float())
}

func (e *reflectedEncoder) encodeString(value reflect.Value, _ int) (err error) {
	return e.writer.EncodeString(value.String())
}

func (e *reflectedEncoder) encodeBytes(value reflect.Value, _ int) (err error) {
	return e.writer.EncodeBytes(value.Bytes())
}

func (e *reflectedEncoder) encodeUnsupported(_ reflect.Value, _ int) (err error) {
	return e.fail(ErrUnsupportedType)
}

func (e *reflectedEncoder) encodeArray(value reflect.Value, element *reflectionPlan, depth int) (err error) {
	if value.Kind() == reflect.Slice && value.IsNil() {
		return e.writer.EncodeNil()
	}

	if uint64(value.Len()) > math.MaxUint32 {
		return e.fail(ErrOverflow)
	}

	if err = e.writer.EncodeArrayHeader(uint32(value.Len())); err != nil {
		return err
	}

	for index := 0; index < value.Len(); index++ {
		if err = e.encodePlanned(value.Index(index), element, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (e *reflectedEncoder) fail(err error) (out error) {
	if e.writer.state.err == nil {
		e.writer.state.err = err
	}

	return e.writer.state.err
}

func (e *reflectedEncoder) encodeMap(value reflect.Value, key, element *reflectionPlan, depth int) (err error) {
	if value.IsNil() {
		return e.writer.EncodeNil()
	}

	if uint64(value.Len()) > math.MaxUint32 {
		return e.fail(ErrOverflow)
	}

	if err = e.writer.EncodeMapHeader(uint32(value.Len())); err != nil {
		return err
	}

	var entries = value.MapRange()
	for entries.Next() {
		if err = e.encodePlanned(entries.Key(), key, depth+1); err != nil {
			return err
		}

		if err = e.encodePlanned(entries.Value(), element, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func (e *reflectedEncoder) encodeStruct(value reflect.Value, fields []reflectionField, prefix []byte, depth int) (err error) {
	if len(fields) == 0 {
		return e.writer.writeEncoded(prefix)
	}

	for _, field := range fields {
		if err = e.writer.writeEncoded(field.prefix); err != nil {
			return err
		}

		if err = e.encodePlanned(value.Field(field.index), field.plan, depth+1); err != nil {
			return err
		}
	}
	return nil
}
