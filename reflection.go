package messagepack

import (
	"reflect"
	"sync"
)

const (
	maximumReflectionDepth  = 256
	maximumReflectionVisits = 1048576
)

var reflectionPlans sync.Map

// loadReflectionPlan publishes one initialized plan per type without retaining values.
func loadReflectionPlan(typ reflect.Type) (plan *reflectionPlan) {
	plan = lookupReflectionPlan(typ)
	plan.once.Do(plan.initialize)
	return plan
}

// lookupReflectionPlan links child types without recursively initializing them.
func lookupReflectionPlan(typ reflect.Type) (plan *reflectionPlan) {
	var (
		cached any
		ok     bool
	)

	if cached, ok = reflectionPlans.Load(typ); !ok {
		cached, _ = reflectionPlans.LoadOrStore(typ, newReflectionPlan(typ))
	}

	plan = cached.(*reflectionPlan)
	return plan
}

func supportsReflection(kind reflect.Kind) (supported bool) {
	switch kind {
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64, reflect.String, reflect.Pointer, reflect.Interface,
		reflect.Array, reflect.Slice, reflect.Map, reflect.Struct:
		return true
	default:
		return false
	}
}
