package messagepack

import (
	"reflect"
	"sync"
)

// newReflectionPlan allocates a plan whose initialization is guarded by the cache.
func newReflectionPlan(typ reflect.Type) (out *reflectionPlan) {
	var p reflectionPlan
	p.typ = typ
	return &p
}

// reflectionPlan stores immutable dispatch, hook eligibility, and struct metadata.
type reflectionPlan struct {
	typ               reflect.Type
	once              sync.Once
	kind              reflect.Kind
	nullable          bool
	encodeHook        bool
	encodeAddressHook bool
	decodeHook        bool
	decodeAddressHook bool
	encode            func(*reflectedEncoder, reflect.Value, int) error
	decode            func(*reflectedDecoder, reflect.Value, byte, int) error
	fields            []int
	fieldsByName      map[string]int
}

func (p *reflectionPlan) initialize() {
	p.kind = p.typ.Kind()
	p.nullable = p.kind == reflect.Pointer || p.kind == reflect.Interface || p.kind == reflect.Slice || p.kind == reflect.Map
	p.encodeHook = p.typ.Implements(reflect.TypeFor[Encodee]())
	p.encodeAddressHook = reflect.PointerTo(p.typ).Implements(reflect.TypeFor[Encodee]())
	p.decodeHook = p.typ.Implements(reflect.TypeFor[Decodee]())
	p.decodeAddressHook = reflect.PointerTo(p.typ).Implements(reflect.TypeFor[Decodee]())

	switch p.kind {
	case reflect.Pointer:
		p.initializePointer()
	case reflect.Interface:
		p.encode = (*reflectedEncoder).encodeIndirect
		p.decode = (*reflectedDecoder).decodeInterface
	case reflect.Bool:
		p.encode = (*reflectedEncoder).encodeBool
		p.decode = (*reflectedDecoder).decodeBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		p.encode = (*reflectedEncoder).encodeInt
		p.decode = (*reflectedDecoder).decodeInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		p.encode = (*reflectedEncoder).encodeUint
		p.decode = (*reflectedDecoder).decodeUint
	case reflect.Float32:
		p.encode = (*reflectedEncoder).encodeFloat32
		p.decode = (*reflectedDecoder).decodeFloat
	case reflect.Float64:
		p.encode = (*reflectedEncoder).encodeFloat64
		p.decode = (*reflectedDecoder).decodeFloat
	case reflect.String:
		p.encode = (*reflectedEncoder).encodeString
		p.decode = (*reflectedDecoder).decodeString
	case reflect.Array, reflect.Slice:
		p.initializeArray()
	case reflect.Map:
		p.initializeMap()
	case reflect.Struct:
		p.initializeStruct()
	default:
		p.encode = (*reflectedEncoder).encodeUnsupported
		p.decode = (*reflectedDecoder).decodeUnsupported
	}
}

func (p *reflectionPlan) initializePointer() {
	p.encode = (*reflectedEncoder).encodeIndirect
	p.decode = (*reflectedDecoder).decodePointer
}

func (p *reflectionPlan) initializeArray() {
	if p.kind == reflect.Slice && p.typ.Elem().Kind() == reflect.Uint8 {
		p.encode = (*reflectedEncoder).encodeBytes
		p.decode = (*reflectedDecoder).decodeBytes
		return
	}

	p.encode = (*reflectedEncoder).encodeArray
	p.decode = (*reflectedDecoder).decodeArray
}

func (p *reflectionPlan) initializeMap() {
	p.encode = (*reflectedEncoder).encodeMap
	p.decode = (*reflectedDecoder).decodeMap
}

func (p *reflectionPlan) initializeStruct() {
	p.fieldsByName = make(map[string]int)
	for index := 0; index < p.typ.NumField(); index++ {
		var field = p.typ.Field(index)
		if field.PkgPath != "" {
			continue
		}

		p.fields = append(p.fields, index)
		p.fieldsByName[field.Name] = index
	}
	p.encode = func(e *reflectedEncoder, value reflect.Value, depth int) (err error) {
		return e.encodeStruct(value, p.fields, depth)
	}
	p.decode = func(d *reflectedDecoder, value reflect.Value, _ byte, depth int) (err error) {
		return d.decodeStruct(value, p.fieldsByName, depth)
	}
}
