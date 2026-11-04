package messagepack

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"time"
)

func ExampleReader_httpResponse() {
	const length = 1024*1024 + 7
	var (
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		produced    = make(chan error, 1)
		message     applicationMessage
		n           uint64
		sink        streamCheckingWriter
	)

	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		produced <- encodeStreamingPayloadMessage(NewWriter(w), length, &encoderGeneratedReader{length: length})
	}))
	defer server.Close()
	var (
		request *http.Request
		err     error
	)

	if request, err = http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil); err != nil {
		fmt.Println("request:", err)
		return
	}

	var (
		response    *http.Response
		responseErr error
	)

	if response, responseErr = server.Client().Do(request); responseErr != nil {
		fmt.Println("response:", responseErr)
		return
	}

	// Decode directly from the body. The caller owns context, limits, and closure.
	d := NewReader(response.Body)
	err = d.SetLimits(ReaderLimits{MaxPayloadBytes: 16})
	if err == nil {
		// The strict six-field helper uses DecodeBytesTo for data; small fields are owned.
		message, n, err = decodeStreamingPayloadMessage(d, &sink)
	}

	closeErr := response.Body.Close()
	producerErr := <-produced
	if err != nil || closeErr != nil || producerErr != nil {
		fmt.Println("decode/close/producer:", err, closeErr, producerErr)
		return
	}

	fmt.Printf("%d %s %d binary bytes %v\n", message.ID, message.Name, n, message.Samples)
	// Output:
	// 7 Ada 1048583 binary bytes [-1 2]
}

func ExampleWriter_file() {
	const (
		count  = 100000
		length = 8*1024*1024 + 7
	)
	var (
		file *os.File
		err  error
	)

	if file, err = os.CreateTemp("", "messagepack-example-*"); err != nil {
		fmt.Println("create:", err)
		return
	}

	path := file.Name()
	defer func() {
		if removeErr := os.Remove(path); removeErr != nil {
			fmt.Println("remove:", removeErr)
		}
	}()
	// The helper writes typed headers/children and generated known-length binary.
	// It owns bufio.Writer, checks encode and Flush separately, and always checks Close.
	if err = writeStreamingDataset(file, count, length); err != nil {
		fmt.Println("write:", err)
		return
	}

	if file, err = os.Open(path); err != nil {
		fmt.Println("open:", err)
		return
	}

	var (
		buffered = bufio.NewReader(file)
		d        = NewReader(buffered)
	)

	err = d.SetLimits(ReaderLimits{MaxPayloadBytes: 1})
	if err == nil {
		err = checkStreamingDataset(d, count, length)
	}

	if err == nil {
		// A finite file permits an EOF check using the same buffered reader.
		_, err = buffered.Peek(1)
		if errors.Is(err, io.EOF) {
			err = nil
		} else if err == nil {
			err = errApplicationTrailing
		}
	}

	closeErr := file.Close()
	if err != nil || closeErr != nil {
		fmt.Println("read/close:", err, closeErr)
		return
	}

	fmt.Printf("checked %d integers and %d binary bytes\n", count, length)
	// Output:
	// checked 100000 integers and 8388615 binary bytes
}

func ExampleReader_messageSequence() {
	var (
		// Each small literal is one complete six-field application message.
		reader = io.MultiReader(bytes.NewReader(applicationMessageFixture()), bytes.NewReader(applicationMessageFixture()))
		d      = NewReader(bufio.NewReader(reader))
		count  int
	)

	for {
		message, err := decodeStreamingApplicationMessage(d)
		if errors.Is(err, io.EOF) {
			break
		}

		if err != nil {
			fmt.Println("decode:", err)
			return
		}

		// Process each message immediately; never check connection EOF per message.
		fmt.Println("id:", message.ID)
		count++
	}
	fmt.Println("complete messages:", count)
	// Output:
	// id: 7
	// id: 7
	// complete messages: 2
}

func ExampleReader_exactlyOneMessage() {
	// Only a finite body or externally bounded frame permits this trailing check.
	buffered := bufio.NewReader(bytes.NewReader(flexibleStreamingApplicationFixture()))
	var (
		message applicationMessage
		err     error
	)

	if message, err = decodeFlexibleStreamingApplicationMessage(NewReader(buffered)); err != nil {
		fmt.Println("decode:", err)
		return
	}

	// Keep using buffered: it may already hold bytes read beyond the message.
	if _, err = buffered.Peek(1); !errors.Is(err, io.EOF) {
		fmt.Println("trailing data or boundary error:", err)
		return
	}

	fmt.Printf("exactly one message: %d %s\n", message.ID, message.Name)
	// Output:
	// exactly one message: 7 Ada
}
