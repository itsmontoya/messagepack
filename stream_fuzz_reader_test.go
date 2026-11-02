package messagepack

import "io"

// streamFuzzReader keeps schedule accounting bounded without retaining read logs.
type streamFuzzReader struct {
	data           []byte
	fragments      []byte
	position       int
	step           int
	empty          int
	emptyBetween   int
	endErr         error
	dataError      bool
	reads          int
	maxRequest     int
	deliveredError error
	noProgress     bool
	emptyRun       int
}

func (r *streamFuzzReader) Read(dst []byte) (n int, err error) {
	r.reads++
	// Even one-byte progress interleaved with 99 empty reads must finish well
	// below this ceiling for every bounded fixture used by these targets.
	if r.reads > 2000000 || len(dst) > 32768 {
		panic("stream fuzz read work/request bound exceeded")
	}
	if len(dst) > r.maxRequest {
		r.maxRequest = len(dst)
	}
	if r.empty > 0 {
		r.empty--
		r.emptyRun++
		if r.emptyRun == 100 {
			r.noProgress = true
		}
		return 0, nil
	}
	if r.position == len(r.data) {
		err = r.endErr
		if err == nil {
			err = io.EOF
		}
		r.deliveredError = err
		return 0, err
	}
	n = len(dst)
	if len(r.fragments) > 0 {
		fragment := 1 + int(r.fragments[r.step%len(r.fragments)]%64)
		r.step++
		if n > fragment {
			n = fragment
		}
	}
	if n > len(r.data)-r.position {
		n = len(r.data) - r.position
	}
	copy(dst[:n], r.data[r.position:r.position+n])
	r.position += n
	r.emptyRun = 0
	r.empty = r.emptyBetween
	if r.position == len(r.data) && r.dataError {
		r.deliveredError = r.endErr
		return n, r.endErr
	}
	return n, nil
}
