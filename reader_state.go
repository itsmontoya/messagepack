package messagepack

import "io"

// newReaderState allocates detached state for a Reader to initialize.
func newReaderState() (out *readerState) {
	var s readerState
	return &s
}

// readerState keeps initialized Reader copies on the same input and failure boundary.
type readerState struct {
	reader io.Reader
	err    error
	eof    bool
	tag    byte
	hasTag bool
	limits ReaderLimits
	buffer []byte
}
