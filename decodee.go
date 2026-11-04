package messagepack

// Decodee defines a value's MessagePack schema using typed Reader operations.
type Decodee interface {
	// DecodeMessagePack populates the receiver from one complete schema value.
	DecodeMessagePack(Reader) error
}
