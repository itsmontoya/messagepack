package messagepack

// allocationBenchmarkValue exercises scalar scratch passed through a hook.
type allocationBenchmarkValue struct {
	value uint64
}

// EncodeMessagePack writes a single scalar without boxing the receiver.
func (v *allocationBenchmarkValue) EncodeMessagePack(w Writer) (err error) {
	return w.EncodeUint64(v.value)
}

// DecodeMessagePack reads a single scalar into the existing receiver.
func (v *allocationBenchmarkValue) DecodeMessagePack(r Reader) (err error) {
	v.value, err = r.DecodeUint64()
	return err
}
