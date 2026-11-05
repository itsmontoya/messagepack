//go:build goexperiment.simd && (arm64 || amd64)

package messagepack

import (
	"bufio"
	"bytes"
	"testing"
)

func TestNumericBatchInputDoesNotFillBuffer(t *testing.T) {
	var (
		source = &decoderTestReader{data: bytes.Repeat([]byte{7}, 64)}
		buffer = bufio.NewReaderSize(source, 32)
		input  = newNumericBatchInput(buffer)
		wire   []byte
		err    error
	)

	if wire, err = input.peek(18); err != nil || len(wire) != 0 || source.reads != 0 {
		t.Fatalf("empty peek = (%x, %v), reads %d", wire, err, source.reads)
	}

	if _, err = buffer.ReadByte(); err != nil {
		t.Fatal(err)
	}

	var reads = source.reads
	if wire, err = input.peek(144); err != nil || len(wire) != 31 || source.reads != reads || buffer.Buffered() != 31 {
		t.Fatalf("buffered peek = (%d bytes, %v), reads %d, buffered %d", len(wire), err, source.reads, buffer.Buffered())
	}

	if err = input.discard(18); err != nil || buffer.Buffered() != 13 || source.reads != reads {
		t.Fatalf("discard = %v, buffered %d, reads %d", err, buffer.Buffered(), source.reads)
	}

	if err = buffer.UnreadByte(); err != nil || buffer.Buffered() != 14 || source.reads != reads {
		t.Fatalf("UnreadByte after batch = %v, buffered %d, reads %d", err, buffer.Buffered(), source.reads)
	}
}
