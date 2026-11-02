package messagepack

import "net/http"

// streamHTTPWriter flushes each write so the response reaches the client before completion.
type streamHTTPWriter struct {
	response http.ResponseWriter
}

func (w *streamHTTPWriter) Write(src []byte) (n int, err error) {
	n, err = w.response.Write(src)
	if err != nil {
		return n, err
	}
	err = http.NewResponseController(w.response).Flush()
	return n, err
}
