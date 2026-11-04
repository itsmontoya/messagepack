package messagepack

// streamBenchmarkWriter counts underlying calls without scanning or retaining output.
type streamBenchmarkWriter struct {
	bytes  uint64
	writes uint64
}

func (w *streamBenchmarkWriter) Write(src []byte) (n int, err error) {
	w.bytes += uint64(len(src))
	w.writes++
	return len(src), nil
}
