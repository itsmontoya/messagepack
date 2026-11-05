package messagepack

import (
	"bytes"
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
	decodeElements    func(*reflectedDecoder, reflect.Value, uint32) error
	fields            []reflectionField
	fieldsByName      map[string]int
	prefix            []byte
	encodeErr         error
	element           *reflectionPlan
	key               *reflectionPlan
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
	p.element = lookupReflectionPlan(p.typ.Elem())
	p.encode = func(e *reflectedEncoder, value reflect.Value, depth int) (err error) {
		return e.encodePointer(value, p.element, depth)
	}
	p.decode = func(d *reflectedDecoder, value reflect.Value, _ byte, depth int) (err error) {
		return d.decodePointer(value, p.element, depth)
	}
}

func (p *reflectionPlan) initializeArray() {
	if p.kind == reflect.Slice && p.typ.Elem().Kind() == reflect.Uint8 {
		p.encode = (*reflectedEncoder).encodeBytes
		p.decode = (*reflectedDecoder).decodeBytes
		return
	}

	p.element = lookupReflectionPlan(p.typ.Elem())
	p.initializeNumericElements()
	p.encode = func(e *reflectedEncoder, value reflect.Value, depth int) (err error) {
		return e.encodeArray(value, p.element, depth)
	}
	p.decode = func(d *reflectedDecoder, value reflect.Value, _ byte, depth int) (err error) {
		return d.decodeArray(value, p.element, p.decodeElements, depth)
	}
}

func (p *reflectionPlan) initializeNumericElements() {
	var element = p.typ.Elem()
	// Inspect the type directly: initializing a child here can deadlock recursive plans.
	if element.Implements(reflect.TypeFor[Decodee]()) || reflect.PointerTo(element).Implements(reflect.TypeFor[Decodee]()) {
		return
	}

	// Named elements retain scalar setters; named collections of these types can batch.
	switch element {
	case reflect.TypeFor[int64]():
		p.decodeElements = (*reflectedDecoder).decodeInt64s
	case reflect.TypeFor[uint64]():
		p.decodeElements = (*reflectedDecoder).decodeUint64s
	case reflect.TypeFor[float64]():
		p.decodeElements = (*reflectedDecoder).decodeFloat64s
	}
}

func (p *reflectionPlan) initializeMap() {
	p.key = lookupReflectionPlan(p.typ.Key())
	p.element = lookupReflectionPlan(p.typ.Elem())
	p.encode = func(e *reflectedEncoder, value reflect.Value, depth int) (err error) {
		return e.encodeMap(value, p.key, p.element, depth)
	}
	p.decode = func(d *reflectedDecoder, value reflect.Value, _ byte, depth int) (err error) {
		return d.decodeMap(value, p.key, p.element, depth)
	}
}

func (p *reflectionPlan) initializeStruct() {
	p.fieldsByName = make(map[string]int)
	for index := 0; index < p.typ.NumField(); index++ {
		var field = p.typ.Field(index)
		if field.PkgPath != "" {
			continue
		}

		p.fields = append(p.fields, reflectionField{index: index, plan: lookupReflectionPlan(field.Type)})
		p.fieldsByName[field.Name] = len(p.fields) - 1
	}
	// Resolve child plans during traversal so recursive types cannot deadlock initialization.
	p.encode = func(e *reflectedEncoder, value reflect.Value, depth int) (err error) {
		if p.encodeErr != nil {
			return e.fail(p.encodeErr)
		}

		return e.encodeStruct(value, p.fields, p.prefix, depth)
	}
	p.decode = func(d *reflectedDecoder, value reflect.Value, _ byte, depth int) (err error) {
		return d.decodeStruct(value, p.fields, p.fieldsByName, depth)
	}
	p.initializePrefixes()
}

func (p *reflectionPlan) initializePrefixes() {
	var (
		prefix bytes.Buffer
		writer = NewWriter(&prefix)
	)

	if p.encodeErr = writer.EncodeMapHeader(uint32(len(p.fields))); p.encodeErr != nil {
		return
	}

	if len(p.fields) == 0 {
		p.prefix = append([]byte(nil), prefix.Bytes()...)
		return
	}

	for index := range p.fields {
		var field = p.typ.Field(p.fields[index].index)
		if p.encodeErr = writer.EncodeString(field.Name); p.encodeErr != nil {
			return
		}

		p.fields[index].prefix = append([]byte(nil), prefix.Bytes()...)
		prefix.Reset()
	}
}
