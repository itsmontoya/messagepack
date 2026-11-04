package messagepack

import "io"

// readerState keeps initialized Reader copies on the same input and failure boundary.
type readerState struct {
	reader io.Reader
	err    error
	eof    bool
	limits ReaderLimits
	buffer []byte
}
