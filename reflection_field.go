package messagepack

// reflectionField binds a struct field to cached wire metadata and its type plan.
type reflectionField struct {
	index  int
	prefix []byte
	plan   *reflectionPlan
}
