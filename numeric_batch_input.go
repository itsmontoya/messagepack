//go:build goexperiment.simd && (arm64 || amd64)

package messagepack

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
)

const numericBatchElements = 16

func newNumericBatchInput(source io.Reader) (out numericBatchInput) {
	out.source = source
	return out
}

// numericBatchInput inspects available memory without advancing or filling a stream.
type numericBatchInput struct {
	source  io.Reader
	scratch [numericBatchElements * 9]byte
}

func (b *numericBatchInput) peek(size int) (wire []byte, err error) {
	switch source := b.source.(type) {
	case *bytes.Buffer:
		return source.Bytes()[:min(size, source.Len())], nil
	case *bytes.Reader:
		size = min(size, source.Len(), len(b.scratch))
		if size == 0 {
			return nil, nil
		}

		wire = b.scratch[:size]
		if _, err = source.ReadAt(wire, source.Size()-int64(source.Len())); err != nil {
			return nil, fmt.Errorf("messagepack: inspect numeric batch source: %w", err)
		}

		return wire, nil
	case *bufio.Reader:
		size = min(size, source.Buffered())
		if wire, err = source.Peek(size); err != nil {
			return nil, fmt.Errorf("messagepack: inspect numeric batch source: %w", err)
		}

		return wire, nil
	default:
		return nil, nil
	}
}

func (b *numericBatchInput) discard(size int) (err error) {
	switch source := b.source.(type) {
	case *bytes.Buffer:
		source.Next(size)
		return nil
	case *bytes.Reader:
		_, err = source.Seek(int64(size), io.SeekCurrent)
	case *bufio.Reader:
		// Preserve the final Read's byte-scanner state without filling the buffer.
		if _, err = source.Discard(size - 1); err == nil {
			_, err = source.ReadByte()
		}
	default:
		return ErrUnsupportedType
	}

	if err != nil {
		return fmt.Errorf("messagepack: consume numeric batch source: %w", err)
	}

	return nil
}
