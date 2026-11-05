package messagepack

// allocationBenchmarkNoop measures dispatch without exposing scratch to I/O.
type allocationBenchmarkNoop struct{}

// EncodeMessagePack returns without writing a value to isolate dispatch overhead.
func (v *allocationBenchmarkNoop) EncodeMessagePack(w Writer) (err error) {
	return nil
}

// DecodeMessagePack returns without reading a value to isolate dispatch overhead.
func (v *allocationBenchmarkNoop) DecodeMessagePack(r Reader) (err error) {
	return nil
}
