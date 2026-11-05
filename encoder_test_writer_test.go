package messagepack

// encoderTestWriter records accepted bytes and exposes short/error writes.
type encoderTestWriter struct {
	limit      int
	limited    bool
	err        error
	data       []byte
	writes     int
	maxRequest int
	flushes    int
	closes     int
}

// Write records accepted prefixes and returns the configured destination failure.
// A typed nil receiver accepts bytes to exercise interface initialization.
func (w *encoderTestWriter) Write(src []byte) (n int, err error) {
	if w == nil {
		return len(src), nil
	}

	w.writes++
	if len(src) > w.maxRequest {
		w.maxRequest = len(src)
	}

	n = len(src)
	if w.limited && n > w.limit {
		n = w.limit
	}

	w.data = append(w.data, src[:n]...)
	return n, w.err
}

// Flush records calls so tests can detect implicit codec buffering.
func (w *encoderTestWriter) Flush() (err error) {
	w.flushes++
	return nil
}

// Close records calls so tests can detect ownership changes.
func (w *encoderTestWriter) Close() (err error) {
	w.closes++
	return nil
}
