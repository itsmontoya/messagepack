package messagepack

// streamFuzzWriter records only bounded output and can fail at any accepted byte.
type streamFuzzWriter struct {
	data       []byte
	fail       bool
	failAt     int
	err        error
	hit        bool
	writes     int
	maxRequest int
}

func (w *streamFuzzWriter) Write(src []byte) (n int, err error) {
	w.writes++
	if w.writes > 20000 || len(src) > 32768 || len(w.data)+len(src) > 20000 {
		panic("stream fuzz write work/output bound exceeded")
	}
	if len(src) > w.maxRequest {
		w.maxRequest = len(src)
	}
	n = len(src)
	if w.fail && len(w.data)+n >= w.failAt {
		n = w.failAt - len(w.data)
		err = w.err
		w.hit = n < len(src) || err != nil
	}
	w.data = append(w.data, src[:n]...)
	return n, err
}
