package messagepack

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeStreamingDataset owns the file lifetime and its optional buffering.
// The array schema is [generated integers, known-length binary].
func writeStreamingDataset(dst io.WriteCloser, count uint32, length uint64) (err error) {
	defer func() {
		closeErr := dst.Close()
		if closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close dataset: %w", closeErr))
		}
	}()
	buffered := bufio.NewWriterSize(dst, 4096)
	if err = encodeStreamingDataset(NewWriter(buffered), count, length); err != nil {
		return fmt.Errorf("encode dataset: %w", err)
	}
	if err = buffered.Flush(); err != nil {
		return fmt.Errorf("flush dataset: %w", err)
	}
	return nil
}

func encodeStreamingDataset(e *Writer, count uint32, length uint64) (err error) {
	if err = e.EncodeArrayHeader(2); err != nil {
		return err
	}
	if err = e.EncodeArrayHeader(count); err != nil {
		return err
	}
	for index := uint64(0); index < uint64(count); index++ {
		if err = e.EncodeUint64(index); err != nil {
			return err
		}
	}
	_, err = e.EncodeBytesFrom(length, &encoderGeneratedReader{length: length})
	return err
}

func checkStreamingDataset(d *Decoder, count uint32, length uint64) (err error) {
	var (
		actual uint32
		value  uint64
		n      uint64
		sink   streamCheckingWriter
	)
	actual, err = d.DecodeArrayHeader()
	if err != nil {
		return err
	}
	if actual != 2 {
		return errApplicationSchema
	}
	actual, err = d.DecodeArrayHeader()
	if err != nil {
		return streamingApplicationChildError(err)
	}
	if actual != count {
		return errApplicationSchema
	}
	for index := uint64(0); index < uint64(count); index++ {
		value, err = d.DecodeUint64()
		if err != nil {
			return streamingApplicationChildError(err)
		}
		if value != index {
			return fmt.Errorf("example: dataset element %d = %d", index, value)
		}
	}
	n, err = d.DecodeBytesTo(&sink)
	if err != nil {
		return streamingApplicationChildError(err)
	}
	if n != length || sink.bytes != length || sink.maxRequest > 32768 {
		return fmt.Errorf("example: dataset payload = %d bytes, want %d", n, length)
	}
	return nil
}

func TestStreamingTemporaryFileDataset(t *testing.T) {
	const (
		count  = 100000
		length = 8*1024*1024 + 7
	)
	var (
		path = filepath.Join(t.TempDir(), "dataset.msgpack")
		file *os.File
		err  error
	)
	file, err = os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = writeStreamingDataset(file, count, length); err != nil {
		t.Fatal(err)
	}
	file, err = os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if file != nil {
			if closeErr := file.Close(); closeErr != nil {
				t.Errorf("close dataset: %v", closeErr)
			}
		}
	})
	var (
		buffered = bufio.NewReaderSize(file, 4096)
		d        = NewDecoder(buffered)
	)
	if err = d.SetLimits(DecoderLimits{MaxPayloadBytes: 1}); err != nil {
		t.Fatal(err)
	}
	if err = checkStreamingDataset(d, count, length); err != nil {
		t.Fatal(err)
	}
	// This is a finite file: the caller checks EOF on the same buffered reader.
	if _, err = buffered.Peek(1); !errors.Is(err, io.EOF) {
		t.Fatalf("dataset trailing data = %v", err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	file = nil
}

func TestStreamingFileErrors(t *testing.T) {
	var (
		writeFailure = errors.New("test: file write failure")
		closeFailure = errors.New("test: file close failure")
	)
	for _, tt := range []struct {
		name     string
		count    uint32
		writeErr error
		closeErr error
		context  string
	}{
		{name: "encode", count: 100000, writeErr: writeFailure, context: "encode dataset"},
		{name: "flush", writeErr: writeFailure, context: "flush dataset"},
		{name: "close", closeErr: closeFailure, context: "close dataset"},
		{name: "encode and close", count: 100000, writeErr: writeFailure, closeErr: closeFailure, context: "encode dataset"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var (
				file = &streamFileWriter{writeErr: tt.writeErr, closeErr: tt.closeErr}
				err  = writeStreamingDataset(file, tt.count, 0)
			)
			if err == nil || (tt.writeErr != nil && !errors.Is(err, tt.writeErr)) ||
				(tt.closeErr != nil && !errors.Is(err, tt.closeErr)) || file.closes != 1 {
				t.Fatalf("file failure = %v, closes %d", err, file.closes)
			}
			if !strings.Contains(err.Error(), tt.context) {
				t.Fatalf("file failure lacks %q context: %v", tt.context, err)
			}
		})
	}
}
