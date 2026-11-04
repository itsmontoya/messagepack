package messagepack

import "context"

// streamGatedReader withholds everything after one chunk until the caller releases it.
type streamGatedReader struct {
	source  encoderGeneratedReader
	ctx     context.Context
	release <-chan struct{}
}

// Read blocks after the first chunk until released or the context ends.
func (r *streamGatedReader) Read(dst []byte) (n int, err error) {
	if r.source.read >= 32768 {
		select {
		case <-r.release:
		case <-r.ctx.Done():
			return 0, r.ctx.Err()
		}
	}

	if r.source.read < 32768 && len(dst) > int(32768-r.source.read) {
		dst = dst[:int(32768-r.source.read)]
	}

	return r.source.Read(dst)
}
