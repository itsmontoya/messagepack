package messagepack

// decoderTestDecodee records dispatch and supplies explicit schema operations.
type decoderTestDecodee struct {
	decode func(r Reader) (err error)
	calls  int
}

func (v *decoderTestDecodee) DecodeMessagePack(r Reader) (err error) {
	if v == nil {
		return r.DecodeNil()
	}
	v.calls++
	return v.decode(r)
}
