package messagepack

import "bytes"

// encoderCountingWriter validates generated payload bytes without retaining them.
type encoderCountingWriter struct {
	header                 []byte
	source                 *encoderGeneratedReader
	bytes                  uint64
	firstPayloadSourceRead uint64
	writes                 int
	maxRequest             int
	invalid                bool
}

func (w *encoderCountingWriter) Write(src []byte) (n int, err error) {
	w.writes++
	if len(src) > w.maxRequest {
		w.maxRequest = len(src)
	}
	if w.writes == 1 {
		w.invalid = !bytes.Equal(src, w.header)
		return len(src), nil
	}
	if w.writes == 2 {
		w.firstPayloadSourceRead = w.source.read
	}
	for index, value := range src {
		if value != byte(w.bytes+uint64(index)) {
			w.invalid = true
		}
	}
	w.bytes += uint64(len(src))
	return len(src), nil
}
