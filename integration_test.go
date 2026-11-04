package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
)

func TestApplicationMessageEncoding(t *testing.T) {
	var (
		message = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
			Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
		wire  = applicationMessageFixture()
		tests = []struct {
			name string
			dst  []byte
		}{
			{name: "fresh"},
			{name: "existing prefix", dst: []byte{0xaa, 0xbb}},
		}
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				want   = append(append([]byte(nil), tt.dst...), wire...)
				output = bytes.NewBuffer(tt.dst)
				err    error
			)

			err = NewEncoder(output).Encode(message)
			if err != nil || !bytes.Equal(output.Bytes(), want) {
				t.Fatalf("Encode() = (%x, %v), want %x", output.Bytes(), err, want)
			}
		})
	}
}

func TestApplicationMessageDecoding(t *testing.T) {
	for _, trailing := range [][]byte{nil, {0xc3, 0xc1}} {
		var (
			fixture = applicationMessageFixture()
			src     = append(append([]byte(nil), fixture...), trailing...)
			before  = append([]byte(nil), src...)
			message applicationMessage
			n       int
			err     error
		)

		message, n, err = readApplicationMessage(src)
		if err != nil || n != len(fixture) {
			t.Fatalf("readApplicationMessage() = (%+v, %d, %v)", message, n, err)
		}

		if message.ID != 7 || message.Name != "Ada" || !message.Active || message.Score != 1.5 ||
			!bytes.Equal(message.Payload, []byte{1, 2}) || message.Samples != [2]int64{-1, 2} {
			t.Fatalf("decoded unexpected message: %+v", message)
		}

		if cap(message.Payload) != len(message.Payload) || !bytes.Equal(src, before) ||
			!bytes.Equal(src[n:], trailing) {
			t.Fatal("decoding modified input, consumed trailing data, or returned an uncapped view")
		}

		message, err = readCompleteApplicationMessage(src)
		if len(trailing) == 0 {
			if err != nil || message.ID != 7 {
				t.Fatalf("complete-message read failed: %+v, %v", message, err)
			}

			continue
		}

		if !errors.Is(err, errApplicationTrailing) {
			t.Fatalf("complete-message read returned %v, want unwanted-trailing-data error", err)
		}
	}
}

func TestApplicationMessageInvalidSchema(t *testing.T) {
	var (
		wrongArrayCount = applicationMessageFixture()
		wrongSampleType = applicationMessageFixture()
	)

	wrongArrayCount[len(wrongArrayCount)-3] = 0x91
	wrongSampleType[len(wrongSampleType)-1] = 0xc0
	tests := []struct {
		name string
		src  []byte
		err  error
	}{
		{name: "wrong map count", src: []byte{0x85}, err: errApplicationSchema},
		{name: "extra map field", src: []byte{0x87}, err: errApplicationSchema},
		{name: "unexpected key", src: []byte{0x86, 0xa2, 'n', 'o', 0x07}, err: errApplicationSchema},
		{name: "reordered key", src: []byte{0x86, 0xa4, 'n', 'a', 'm', 'e'}, err: errApplicationSchema},
		{name: "duplicate key", src: []byte{0x86, 0xa2, 'i', 'd', 0x07, 0xa2, 'i', 'd'}, err: errApplicationSchema},
		{name: "wrong id type", src: []byte{0x86, 0xa2, 'i', 'd', 0xc0}, err: ErrTypeMismatch},
		{name: "wrong array count", src: wrongArrayCount, err: errApplicationSchema},
		{name: "wrong sample type", src: wrongSampleType, err: ErrTypeMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				before  = append([]byte(nil), tt.src...)
				message applicationMessage
				n       int
				err     error
			)

			message, n, err = readApplicationMessage(tt.src)
			if !errors.Is(err, tt.err) || message.ID != 0 || message.Name != "" ||
				message.Active || message.Score != 0 || message.Payload != nil || message.Samples != [2]int64{} {
				t.Fatalf("readApplicationMessage() = (%+v, %d, %v), want zero and %v", message, n, err, tt.err)
			}

			if !bytes.Equal(tt.src, before) {
				t.Fatal("fixed-schema decoder modified input")
			}
		})
	}
}

func TestApplicationMessageTruncated(t *testing.T) {
	wire := applicationMessageFixture()
	for length := 0; length < len(wire); length++ {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			var (
				before  = append([]byte(nil), wire...)
				message applicationMessage
				n       int
				err     error
			)

			message, n, err = readApplicationMessage(wire[:length])
			if !errors.Is(err, streamFuzzError(errFuzzTruncated, length)) || message.ID != 0 || message.Name != "" ||
				message.Active || message.Score != 0 || message.Payload != nil || message.Samples != [2]int64{} {
				t.Fatalf("readApplicationMessage() = (%+v, %d, %v), want zero and errFuzzTruncated", message, n, err)
			}

			if !bytes.Equal(wire, before) {
				t.Fatal("truncated read modified source backing storage")
			}
		})
	}
}

func TestApplicationMessageOwnershipAndReuse(t *testing.T) {
	var (
		wire    = applicationMessageFixture()
		message applicationMessage
		out     []byte
		err     error
	)

	if message, err = readCompleteApplicationMessage(wire); err != nil {
		t.Fatal(err)
	}

	owned := append([]byte(nil), message.Payload...)
	message.Payload[0] = 9
	if !bytes.Equal(wire, applicationMessageFixture()) || !bytes.Equal(owned, []byte{1, 2}) {
		t.Fatal("owned payload mutation changed input or its independent copy")
	}

	// Owned payload storage remains valid while the input buffer is reused.
	message.Payload = owned
	message.Name = "Eve"
	output := bytes.NewBuffer(wire[:0])
	err = NewEncoder(output).Encode(message)
	out = output.Bytes()
	if err != nil || &out[0] != &wire[0] {
		t.Fatalf("reuse failed: %v", err)
	}

	if !bytes.Equal(owned, []byte{1, 2}) {
		t.Fatal("source reuse changed the copied payload")
	}
}
