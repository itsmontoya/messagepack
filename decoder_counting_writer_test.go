package messagepack

// decoderCountingWriter checks a generated payload without retaining its bytes.
type decoderCountingWriter struct {
	source                 *encoderGeneratedReader
	bytes                  uint64
	firstPayloadSourceRead uint64
	writes                 int
	maxRequest             int
	invalid                bool
}

// Write validates payload progress and records transfer request sizes.
func (w *decoderCountingWriter) Write(src []byte) (n int, err error) {
	w.writes++
	if w.writes == 1 {
		w.firstPayloadSourceRead = w.source.read
	}

	if len(src) > w.maxRequest {
		w.maxRequest = len(src)
	}

	for index, value := range src {
		if value != byte(w.bytes+uint64(index)) {
			w.invalid = true
		}
	}
	w.bytes += uint64(len(src))
	return len(src), nil
}
