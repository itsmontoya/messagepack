package messagepack

import "io"

// streamOwnedBody records explicit caller closure without changing body reads.
type streamOwnedBody struct {
	body   io.ReadCloser
	closes int
}

func (b *streamOwnedBody) Read(dst []byte) (n int, err error) {
	return b.body.Read(dst)
}

func (b *streamOwnedBody) Close() (err error) {
	b.closes++
	return b.body.Close()
}
