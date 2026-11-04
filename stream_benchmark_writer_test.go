package messagepack

// streamBenchmarkWriter counts underlying calls without scanning or retaining output.
type streamBenchmarkWriter struct {
	bytes  uint64
	writes uint64
}

// Write counts accepted bytes and calls without scanning or retaining payload data.
func (w *streamBenchmarkWriter) Write(src []byte) (n int, err error) {
	w.bytes += uint64(len(src))
	w.writes++
	return len(src), nil
}
