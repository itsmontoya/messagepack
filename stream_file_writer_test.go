package messagepack

// streamFileWriter injects file write and close failures without retaining output.
type streamFileWriter struct {
	writeErr error
	closeErr error
	bytes    uint64
	closes   int
}

// Write injects a file failure or counts accepted bytes without storing output.
func (w *streamFileWriter) Write(src []byte) (n int, err error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}

	w.bytes += uint64(len(src))
	return len(src), nil
}

// Close records caller ownership actions and returns the configured failure.
func (w *streamFileWriter) Close() (err error) {
	w.closes++
	return w.closeErr
}
