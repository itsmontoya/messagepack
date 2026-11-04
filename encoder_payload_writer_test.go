package messagepack

// encoderPayloadWriter fails a selected write after accepting a chosen prefix.
type encoderPayloadWriter struct {
	data       []byte
	failCall   int
	accept     int
	acceptAll  bool
	err        error
	writes     int
	maxRequest int
	flushes    int
	closes     int
}

// Write injects failure on the selected call after accepting its configured prefix.
func (w *encoderPayloadWriter) Write(src []byte) (n int, err error) {
	w.writes++
	if len(src) > w.maxRequest {
		w.maxRequest = len(src)
	}

	n = len(src)
	if w.writes == w.failCall {
		if !w.acceptAll && n > w.accept {
			n = w.accept
		}

		err = w.err
	}

	w.data = append(w.data, src[:n]...)
	return n, err
}

// Flush records calls so tests can detect implicit codec buffering.
func (w *encoderPayloadWriter) Flush() (err error) {
	w.flushes++
	return nil
}

// Close records calls so tests can detect ownership changes.
func (w *encoderPayloadWriter) Close() (err error) {
	w.closes++
	return nil
}
