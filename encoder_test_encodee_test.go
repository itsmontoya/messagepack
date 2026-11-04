package messagepack

// encoderTestEncodee records dispatch and supplies explicit schema operations.
type encoderTestEncodee struct {
	encode func(w Writer) (err error)
	calls  int
}

func (v *encoderTestEncodee) EncodeMessagePack(w Writer) (err error) {
	if v == nil {
		return w.EncodeNil()
	}
	v.calls++
	return v.encode(w)
}
