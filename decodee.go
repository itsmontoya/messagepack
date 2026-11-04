package messagepack

// Decodee defines a value's MessagePack schema using typed Reader operations.
type Decodee interface {
	DecodeMessagePack(Reader) error
}
