package messagepack

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

func TestReflectionPlanCache(t *testing.T) {
	type first struct{ ID uint64 }
	type second struct{ ID uint64 }
	var (
		typ    = reflect.TypeFor[first]()
		plan   = loadReflectionPlan(typ)
		cached *reflectionPlan
		allocs = testing.AllocsPerRun(100, func() {
			cached = loadReflectionPlan(typ)
		})
	)

	if cached != plan || allocs != 0 {
		t.Fatalf("cached lookup = %p, %.0f allocations, want (%p, 0)", cached, allocs, plan)
	}

	if loadReflectionPlan(reflect.TypeFor[second]()) == plan {
		t.Fatal("distinct named types share a reflection plan")
	}

	if loadReflectionPlan(reflect.TypeFor[*first]()) == plan {
		t.Fatal("pointer and value types share a reflection plan")
	}
}

func TestReflectionPlanConcurrentRecursiveStruct(t *testing.T) {
	type node struct {
		ID       uint64
		Signed   [2]int64
		Unsigned []uint64
		Floats   []float64
		Children []node
		Next     *node
	}
	type result struct {
		plan *reflectionPlan
		err  error
	}
	var (
		start   = make(chan struct{})
		results = make(chan result, 32)
		plan    *reflectionPlan
	)

	for index := uint64(0); index < 32; index++ {
		go func(id uint64) {
			<-start
			var (
				cached = loadReflectionPlan(reflect.TypeFor[node]())
				value  = node{ID: id, Signed: [2]int64{-1, 2}, Unsigned: []uint64{3, 4}, Floats: []float64{1.5, 2.5}, Children: []node{{ID: id + 2}}, Next: &node{ID: id + 1}}
				output bytes.Buffer
				target node
				err    error
			)

			if err = NewEncoder(&output).Encode(&value); err != nil {
				results <- result{plan: cached, err: err}
				return
			}

			if err = NewDecoder(&output).Decode(&target); err != nil {
				results <- result{plan: cached, err: err}
				return
			}

			if !reflect.DeepEqual(target, value) || output.Len() != 0 {
				err = fmt.Errorf("decoded %+v, want %+v, remaining %d", target, value, output.Len())
			}

			results <- result{plan: cached, err: err}
		}(index)
	}
	close(start)
	for index := 0; index < 32; index++ {
		var completed = <-results
		if completed.err != nil {
			t.Error(completed.err)
		}

		if plan == nil {
			plan = completed.plan
		}

		if completed.plan != plan {
			t.Errorf("concurrent lookup = %p, want %p", completed.plan, plan)
		}
	}
}

func TestReflectionPlanRecursiveCollections(t *testing.T) {
	type recursiveSlice []recursiveSlice
	type recursiveArray [1]*recursiveArray
	tests := []struct {
		typ  reflect.Type
		wire []byte
	}{
		{typ: reflect.TypeFor[recursiveSlice](), wire: []byte{0x91, 0xc0}},
		{typ: reflect.TypeFor[recursiveArray](), wire: []byte{0x91, 0xc0}},
	}

	for _, tt := range tests {
		var (
			target = reflect.New(tt.typ)
			err    = NewDecoder(bytes.NewReader(tt.wire)).Decode(target.Interface())
		)

		if err != nil {
			t.Fatalf("recursive %s: %v", tt.typ, err)
		}
	}
}

func TestReflectionPlanNumericElements(t *testing.T) {
	type namedSlice []int64
	type namedNumber int64
	tests := []struct {
		typ   reflect.Type
		batch bool
	}{
		{typ: reflect.TypeFor[[2]int64](), batch: true},
		{typ: reflect.TypeFor[[]uint64](), batch: true},
		{typ: reflect.TypeFor[[]float64](), batch: true},
		{typ: reflect.TypeFor[namedSlice](), batch: true},
		{typ: reflect.TypeFor[[]namedNumber]()},
		{typ: reflect.TypeFor[[]reflectionValueNumber]()},
		{typ: reflect.TypeFor[[]reflectionPointerNumber]()},
		{typ: reflect.TypeFor[[]*int64]()},
		{typ: reflect.TypeFor[[]any]()},
		{typ: reflect.TypeFor[[]int32]()},
		{typ: reflect.TypeFor[[]float32]()},
		{typ: reflect.TypeFor[[]byte]()},
	}

	for _, tt := range tests {
		var plan = loadReflectionPlan(tt.typ)
		if (plan.decodeElements != nil) != tt.batch {
			t.Errorf("batch handler for %s = %v, want %v", tt.typ, plan.decodeElements != nil, tt.batch)
		}
	}
}
