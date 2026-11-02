package messagepack

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamingHTTPIncrementalPayload(t *testing.T) {
	const length = 8*1024*1024 + 7
	sinkFailure := errors.New("test: destination rejected payload")
	for _, mode := range []string{"complete", "destination failure", "cancel request"} {
		t.Run(mode, func(t *testing.T) {
			var (
				ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
				release     = make(chan struct{})
				produced    = make(chan error, 1)
				first       = make(chan struct{})
				decoded     = make(chan struct{})
				sink        = &streamCheckingWriter{first: first}
				body        *streamOwnedBody
				message     applicationMessage
				n           uint64
				decodeErr   error
			)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var (
					source = &streamGatedReader{
						source: encoderGeneratedReader{length: length}, ctx: r.Context(), release: release,
					}
					err error
				)
				// A blocked socket write must also have a caller-controlled deadline.
				err = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err == nil {
					err = encodeStreamingPayloadMessage(NewEncoder(&streamHTTPWriter{response: w}), length, source)
				}
				produced <- err
			}))
			t.Cleanup(func() {
				cancel()
				if body != nil && body.closes == 0 {
					if err := body.Close(); err != nil {
						t.Errorf("cleanup response body: %v", err)
					}
				}
				server.CloseClientConnections()
				server.Close()
			})
			request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			if requestErr != nil {
				t.Fatal(requestErr)
			}
			response, responseErr := server.Client().Do(request)
			if responseErr != nil {
				t.Fatal(responseErr)
			}
			body = &streamOwnedBody{body: response.Body}
			d := NewDecoder(body)
			if err := d.SetLimits(DecoderLimits{MaxPayloadBytes: 16}); err != nil {
				t.Fatal(err)
			}
			if mode == "destination failure" {
				sink.failAfter, sink.failure = 3, sinkFailure
			}
			go func() {
				message, n, decodeErr = decodeStreamingPayloadMessage(d, sink)
				close(decoded)
			}()
			// Closing the body and canceling the request releases the decoder on every exit.
			t.Cleanup(func() {
				cancel()
				if body.closes == 0 {
					if err := body.Close(); err != nil {
						t.Errorf("cleanup response body: %v", err)
					}
				}
				<-decoded
			})
			select {
			case <-first:
			case <-ctx.Done():
				t.Fatalf("waiting for incremental payload: %v", ctx.Err())
			}
			select {
			case producerErr := <-produced:
				t.Fatalf("producer finished before gate release: %v", producerErr)
			default:
			}
			if mode == "complete" {
				close(release)
			}
			if mode == "cancel request" {
				cancel()
			}
			select {
			case <-decoded:
			case <-ctx.Done():
				if mode != "cancel request" {
					t.Fatalf("waiting for decoder: %v", ctx.Err())
				}
				<-decoded
			}
			if body.closes != 0 {
				t.Fatal("decoder closed the caller's response body")
			}
			if err := body.Close(); err != nil {
				t.Fatalf("close response body: %v", err)
			}
			cancel()
			producerErr := <-produced
			switch mode {
			case "complete":
				if decodeErr != nil || producerErr != nil || n != length || sink.bytes != length ||
					message.ID != 7 || message.Name != "Ada" || !message.Active || message.Score != 1.5 ||
					message.Payload != nil || message.Samples != [2]int64{-1, 2} {
					t.Fatalf("HTTP message = (%+v, %d, %v), producer %v, sink %d", message, n, decodeErr, producerErr, sink.bytes)
				}
			case "destination failure":
				if !errors.Is(decodeErr, sinkFailure) || n != 3 || sink.bytes != 3 || sink.writes != 1 {
					t.Fatalf("destination failure = (%d, %v), sink %d/%d writes", n, decodeErr, sink.bytes, sink.writes)
				}
			case "cancel request":
				if !errors.Is(decodeErr, context.Canceled) || n != 32768 || sink.bytes != 32768 {
					t.Fatalf("request cancellation = (%d, %v), sink %d", n, decodeErr, sink.bytes)
				}
			}
			if sink.maxRequest > 32768 {
				t.Fatalf("unbounded sink write: %d", sink.maxRequest)
			}
			if decodeErr != nil && d.DecodeNil() != decodeErr {
				t.Fatal("HTTP transfer failure did not remain terminal")
			}
		})
	}
}

func TestStreamingHTTPTruncatedResponse(t *testing.T) {
	fixture := applicationMessageFixture()
	for _, length := range []int{1, len(fixture) - 1} {
		var (
			ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
			produced    = make(chan error, 1)
		)
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write(fixture[:length])
			produced <- err
		}))
		t.Cleanup(server.Close)
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		response, responseErr := server.Client().Do(request)
		if responseErr != nil {
			cancel()
			t.Fatal(responseErr)
		}
		_, err = decodeStreamingApplicationMessage(NewDecoder(response.Body))
		closeErr := response.Body.Close()
		cancel()
		if !errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) || closeErr != nil {
			t.Fatalf("truncated HTTP schema = %v, close %v", err, closeErr)
		}
		if err = <-produced; err != nil {
			t.Fatal(err)
		}
	}
}

func TestStreamingHTTPTruncatedPayload(t *testing.T) {
	const (
		declared = 1024 * 1024
		actual   = 32768 + 7
	)
	var (
		ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
		produced    = make(chan error, 1)
		sink        streamCheckingWriter
	)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := NewEncoder(w).EncodeBytesFrom(declared, &encoderGeneratedReader{length: actual})
		produced <- err
	}))
	defer server.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, responseErr := server.Client().Do(request)
	if responseErr != nil {
		t.Fatal(responseErr)
	}
	n, decodeErr := NewDecoder(response.Body).DecodeBytesTo(&sink)
	closeErr := response.Body.Close()
	if !errors.Is(decodeErr, io.ErrUnexpectedEOF) || n != actual || sink.bytes != actual || closeErr != nil {
		t.Fatalf("truncated HTTP payload = (%d, %v), sink %d, close %v", n, decodeErr, sink.bytes, closeErr)
	}
	if err = <-produced; !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short HTTP payload source = %v", err)
	}
}

func TestStreamingHTTPCallerDeadline(t *testing.T) {
	// An already expired deadline makes this test deterministic without sleeps.
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("expired request reached the server")
	}))
	defer server.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, responseErr := server.Client().Do(request)
	if response != nil {
		if err = response.Body.Close(); err != nil {
			t.Errorf("close unexpected response: %v", err)
		}
	}
	if !errors.Is(responseErr, context.DeadlineExceeded) {
		t.Fatalf("caller deadline = %v", responseErr)
	}
}
