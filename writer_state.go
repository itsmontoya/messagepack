package messagepack

import "io"

// newWriterState allocates detached state for a Writer to initialize.
func newWriterState() (out *writerState) {
	var s writerState
	return &s
}

// writerState keeps initialized Writer copies on the same output and failure boundary.
type writerState struct {
	writer io.Writer
	err    error
	buffer []byte
}
