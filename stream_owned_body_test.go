package messagepack

import (
	"fmt"
	"io"
)

// streamOwnedBody tracks caller closure while preserving body bytes and EOF.
type streamOwnedBody struct {
	body   io.ReadCloser
	closes int
}

// Read preserves EOF and adds context to response-body read failures.
func (b *streamOwnedBody) Read(dst []byte) (n int, err error) {
	n, err = b.body.Read(dst)
	if err != nil && err != io.EOF {
		return n, fmt.Errorf("read response body: %w", err)
	}

	return n, err
}

// Close records ownership and adds context to response-body close failures.
func (b *streamOwnedBody) Close() (err error) {
	b.closes++
	if err = b.body.Close(); err != nil {
		return fmt.Errorf("close response body: %w", err)
	}

	return nil
}
