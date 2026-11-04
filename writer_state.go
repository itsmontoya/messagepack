package messagepack

import "io"

// writerState keeps initialized Writer copies on the same output and failure boundary.
type writerState struct {
	writer io.Writer
	err    error
	buffer []byte
}
