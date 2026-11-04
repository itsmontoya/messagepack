package messagepack

// skipState bounds one traversal independently of advertised container counts.
type skipState struct {
	remainingBytes  uint64
	remainingValues uint64
	outstanding     uint64
	depth           uint32
	children        [maximumSkipDepth]uint64
	started         bool
}
