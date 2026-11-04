package messagepack

import (
	"fmt"
	"net/http"
)

// streamHTTPWriter flushes each write so the response reaches the client before completion.
type streamHTTPWriter struct {
	response http.ResponseWriter
}

// Write flushes accepted response bytes so tests can observe incremental delivery.
func (w *streamHTTPWriter) Write(src []byte) (n int, err error) {
	if n, err = w.response.Write(src); err != nil {
		return n, fmt.Errorf("write HTTP response: %w", err)
	}

	if err = http.NewResponseController(w.response).Flush(); err != nil {
		return n, fmt.Errorf("flush HTTP response: %w", err)
	}

	return n, nil
}
