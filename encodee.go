package messagepack

// Encodee defines a value's MessagePack schema using typed Writer operations.
type Encodee interface {
	// EncodeMessagePack writes one complete schema value using the supplied Writer.
	EncodeMessagePack(Writer) error
}
