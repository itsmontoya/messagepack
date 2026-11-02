package messagepack

import "fmt"

// streamArrayWriter validates one generated scalar per write without retaining values.
type streamArrayWriter struct {
	count      uint32
	seen       uint64
	bytes      uint64
	writes     int
	maxRequest int
}

func (w *streamArrayWriter) Write(src []byte) (n int, err error) {
	w.writes++
	if len(src) > w.maxRequest {
		w.maxRequest = len(src)
	}
	if w.writes == 1 {
		var count uint32
		count, n, err = ReadArrayHeader(src)
		if err != nil {
			return 0, err
		}
		if count != w.count || n != len(src) {
			return 0, fmt.Errorf("example: unexpected generated array header %x", src)
		}
		w.bytes += uint64(n)
		return n, nil
	}
	var value uint64
	value, n, err = ReadUint64(src)
	if err != nil {
		return 0, err
	}
	if value != w.seen || n != len(src) {
		return 0, fmt.Errorf("example: unexpected generated element %x at %d", src, w.seen)
	}
	w.seen++
	w.bytes += uint64(n)
	return n, nil
}
