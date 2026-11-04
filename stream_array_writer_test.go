package messagepack

import (
	"bytes"
	"fmt"
)

// streamArrayWriter validates one generated scalar per write without retaining values.
type streamArrayWriter struct {
	count      uint32
	seen       uint64
	bytes      uint64
	writes     int
	maxRequest int
	input      bytes.Reader
	reader     *Reader
}

// Write validates each generated header or scalar without retaining the dataset.
func (w *streamArrayWriter) Write(src []byte) (n int, err error) {
	w.input.Reset(src)
	if w.reader == nil {
		w.reader = NewReader(&w.input)
	}

	w.reader.Reset(&w.input)
	w.writes++
	if len(src) > w.maxRequest {
		w.maxRequest = len(src)
	}

	if w.writes == 1 {
		var count uint32
		count, err = w.reader.DecodeArrayHeader()
		n = len(src) - w.input.Len()
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
	value, err = w.reader.DecodeUint64()
	n = len(src) - w.input.Len()
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
