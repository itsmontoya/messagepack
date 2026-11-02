package messagepack

// encoderTestWriter records accepted bytes and exposes short/error writes.
type encoderTestWriter struct {
	limit   int
	limited bool
	err     error
	data    []byte
	writes  int
	flushes int
	closes  int
}

func (w *encoderTestWriter) Write(src []byte) (n int, err error) {
	if w == nil {
		return len(src), nil
	}
	w.writes++
	n = len(src)
	if w.limited && n > w.limit {
		n = w.limit
	}
	w.data = append(w.data, src[:n]...)
	return n, w.err
}

func (w *encoderTestWriter) Flush() (err error) {
	w.flushes++
	return nil
}

func (w *encoderTestWriter) Close() (err error) {
	w.closes++
	return nil
}
