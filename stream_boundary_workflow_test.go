package messagepack

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"
)

// decodeExactlyOneStreamingMessage requires a finite body or externally bounded frame.
func decodeExactlyOneStreamingMessage(buffered *bufio.Reader) (message applicationMessage, err error) {
	message, err = decodeStreamingApplicationMessage(NewReader(buffered))
	if err != nil {
		return applicationMessage{}, err
	}
	// Continue through the same wrapper, which may already hold trailing bytes.
	_, err = buffered.Peek(1)
	if errors.Is(err, io.EOF) {
		return message, nil
	}
	if err != nil {
		return applicationMessage{}, fmt.Errorf("check message boundary: %w", err)
	}
	return applicationMessage{}, errApplicationTrailing
}

func TestStreamingExactlyOneFrame(t *testing.T) {
	fixture := applicationMessageFixture()
	for _, tt := range []struct {
		name string
		wire []byte
		want error
	}{
		{name: "exact", wire: fixture},
		{name: "extra message", wire: append(append([]byte(nil), fixture...), fixture...), want: errApplicationTrailing},
		{name: "extra incomplete value", wire: append(append([]byte(nil), fixture...), 0xc4), want: errApplicationTrailing},
		{name: "missing child", wire: fixture[:len(fixture)-1], want: io.ErrUnexpectedEOF},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var (
				// The sentinel is outside the frame; it must remain available to the caller.
				connection = bufio.NewReader(io.MultiReader(bytes.NewReader(tt.wire), bytes.NewReader([]byte{0xc3})))
				frame      = bufio.NewReader(io.LimitReader(connection, int64(len(tt.wire))))
				message    applicationMessage
				err        error
			)
			message, err = decodeExactlyOneStreamingMessage(frame)
			if !errors.Is(err, tt.want) || (err == nil && message.ID != 7) {
				t.Fatalf("framed message = (%+v, %v), want %v", message, err, tt.want)
			}
			// After rejection, the caller discards the rest of this finite frame explicitly.
			if _, err = io.Copy(io.Discard, frame); err != nil {
				t.Fatalf("discard finite frame remainder: %v", err)
			}
			if value, nextErr := NewReader(connection).DecodeBool(); nextErr != nil || !value {
				t.Fatalf("next frame sentinel = (%v, %v)", value, nextErr)
			}
		})
	}
	finite := bufio.NewReader(bytes.NewReader(fixture))
	if _, err := decodeExactlyOneStreamingMessage(finite); err != nil {
		t.Fatalf("finite body: %v", err)
	}
}

func TestStreamingCallerOwnedBuffering(t *testing.T) {
	var (
		file     encoderPayloadWriter
		buffered = bufio.NewWriterSize(&file, 256)
		e        = NewWriter(buffered)
		message  = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
			Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
	)
	if err := encodeStreamingApplicationMessage(e, message); err != nil {
		t.Fatal(err)
	}
	if len(file.data) != 0 || file.flushes != 0 || file.closes != 0 || buffered.Buffered() == 0 {
		t.Fatal("encoder flushed or closed the caller's buffering")
	}
	if _, err := buffered.WriteString("tail"); err != nil {
		t.Fatal(err)
	}
	if err := buffered.Flush(); err != nil {
		t.Fatal(err)
	}
	var (
		input = bufio.NewReaderSize(bytes.NewReader(file.data), 256)
		d     = NewReader(input)
		tail  [4]byte
	)
	if _, err := decodeStreamingApplicationMessage(d); err != nil {
		t.Fatal(err)
	}
	if input.Buffered() != len(tail) {
		t.Fatalf("caller read-ahead = %d, want %d", input.Buffered(), len(tail))
	}
	if _, err := io.ReadFull(input, tail[:]); err != nil || string(tail[:]) != "tail" {
		t.Fatalf("continued buffered input = %q, %v", tail, err)
	}
	if file.closes != 0 {
		t.Fatal("codec closed the caller's writer")
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestStreamingPipeMessageSequence(t *testing.T) {
	var (
		ctx, cancel    = context.WithTimeout(context.Background(), 10*time.Second)
		reader, writer = io.Pipe()
		release        = make(chan struct{})
		writingSecond  = make(chan struct{})
		finished       = make(chan struct{})
		canceled       = make(chan struct{})
		producerErr    error
		message        = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
			Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
	)
	stop := context.AfterFunc(ctx, func() {
		defer close(canceled)
		if err := reader.CloseWithError(ctx.Err()); err != nil {
			t.Errorf("cancel pipe reader: %v", err)
		}
		if err := writer.CloseWithError(ctx.Err()); err != nil {
			t.Errorf("cancel pipe writer: %v", err)
		}
	})
	go func() {
		e := NewWriter(writer)
		producerErr = encodeStreamingApplicationMessage(e, message)
		if producerErr == nil {
			select {
			case <-release:
				close(writingSecond)
				message.ID = 8
				producerErr = encodeStreamingApplicationMessage(e, message)
			case <-ctx.Done():
				producerErr = ctx.Err()
			}
		}
		closeErr := writer.CloseWithError(producerErr)
		if producerErr == nil {
			producerErr = closeErr
		}
		close(finished)
	}()
	// Cleanup releases blocked pipe operations and joins the producer even on Fatal.
	t.Cleanup(func() {
		if !stop() {
			<-canceled
		}
		cancel()
		if err := reader.Close(); err != nil {
			t.Errorf("close pipe reader: %v", err)
		}
		if err := writer.Close(); err != nil {
			t.Errorf("close pipe writer: %v", err)
		}
		<-finished
	})
	var (
		buffered = bufio.NewReader(reader)
		d        = NewReader(buffered)
	)
	first, firstErr := decodeStreamingApplicationMessage(d)
	if firstErr != nil || first.ID != 7 {
		t.Fatalf("first persistent-stream message = (%+v, %v)", first, firstErr)
	}
	select {
	case <-finished:
		t.Fatal("producer finished before the consumer released its gate")
	default:
	}
	// A complete application message is sufficient; do not wait for connection EOF.
	close(release)
	select {
	case <-writingSecond:
	case <-ctx.Done():
		t.Fatalf("waiting for second message: %v", ctx.Err())
	}
	select {
	case <-finished:
		t.Fatal("pipe producer bypassed backpressure before the second read")
	default:
	}
	second, secondErr := decodeStreamingApplicationMessage(d)
	if secondErr != nil || second.ID != 8 {
		t.Fatalf("second persistent-stream message = (%+v, %v)", second, secondErr)
	}
	if _, err := decodeStreamingApplicationMessage(d); !errors.Is(err, io.EOF) {
		t.Fatalf("sequence termination = %v", err)
	}
	<-finished
	if producerErr != nil {
		t.Fatalf("pipe producer: %v", producerErr)
	}
}
