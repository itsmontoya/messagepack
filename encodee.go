package messagepack

// Encodee defines a value's MessagePack schema using typed Writer operations.
type Encodee interface {
	EncodeMessagePack(Writer) error
}
