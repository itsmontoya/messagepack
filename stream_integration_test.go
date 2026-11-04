package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
)

func TestStreamingApplicationMessageWireAndInteroperability(t *testing.T) {
	var (
		message = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
			Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
		writer  bytes.Buffer
		e       = NewWriter(&writer)
		fixture = applicationMessageFixture()
	)
	if err := encodeStreamingApplicationMessage(e, message); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(writer.Bytes(), fixture) {
		t.Fatalf("stream encoded %x, want literal fixture %x", writer.Bytes(), fixture)
	}

	fromStream, sliceErr := readCompleteApplicationMessage(writer.Bytes())
	if sliceErr != nil || !reflect.DeepEqual(fromStream, message) {
		t.Fatalf("slice read of stream output = (%+v, %v)", fromStream, sliceErr)
	}

	for _, maxRead := range []int{0, 1, 3} {
		r := &decoderTestReader{data: fixture, maxRead: maxRead}
		d := NewReader(r)
		decoded, decodeErr := decodeStreamingApplicationMessage(d)
		if decodeErr != nil || !reflect.DeepEqual(decoded, message) || r.position != len(fixture) {
			t.Fatalf("stream read with fragment %d = (%+v, %v), position %d", maxRead, decoded, decodeErr, r.position)
		}
	}
}

func TestStreamingApplicationMessageSequenceAndOwnership(t *testing.T) {
	var (
		fixture = applicationMessageFixture()
		wire    = append(append([]byte(nil), fixture...), fixture...)
		r       = &decoderTestReader{data: wire, maxRead: 1, endErr: io.EOF}
		d       = NewReader(r)
		first   applicationMessage
		second  applicationMessage
		err     error
	)
	first, err = decodeStreamingApplicationMessage(d)
	if err != nil || r.position != len(fixture) {
		t.Fatalf("first message = (%+v, %v), position %d", first, err, r.position)
	}

	second, err = decodeStreamingApplicationMessage(d)
	if err != nil || !reflect.DeepEqual(first, second) || r.position != len(wire) {
		t.Fatalf("second message = (%+v, %v), position %d", second, err, r.position)
	}

	if _, err = decodeStreamingApplicationMessage(d); !errors.Is(err, io.EOF) {
		t.Fatalf("sequence end = %v, want EOF before next root", err)
	}

	for index := range wire {
		wire[index] = 0
	}
	second.Payload[0] = 9
	d.Reset(bytes.NewReader([]byte{0xa1, 'x'}))
	if _, err = d.DecodeString(); err != nil {
		t.Fatal(err)
	}

	if first.Name != "Ada" || !bytes.Equal(first.Payload, []byte{1, 2}) {
		t.Fatalf("later message, source mutation or Reset changed owned results: %+v", first)
	}
}

func TestStreamingApplicationMessageTruncation(t *testing.T) {
	fixture := applicationMessageFixture()
	for length := 0; length < len(fixture); length++ {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			var (
				r       = &decoderTestReader{data: fixture[:length], maxRead: 1}
				d       = NewReader(r)
				message applicationMessage
				err     error
				want    = io.ErrUnexpectedEOF
			)
			if length == 0 {
				want = io.EOF
			}

			message, err = decodeStreamingApplicationMessage(d)
			if !errors.Is(err, want) || !reflect.DeepEqual(message, applicationMessage{}) || r.position != length {
				t.Fatalf("truncated message = (%+v, %v), position %d, want zero and %v", message, err, r.position, want)
			}

			if length > 0 && errors.Is(err, io.EOF) {
				t.Fatal("EOF inside the schema was mistaken for a complete sequence")
			}
		})
	}
}

func TestStreamingApplicationMessageInvalidSchema(t *testing.T) {
	var (
		wrongArrayCount = applicationMessageFixture()
		wrongSampleType = applicationMessageFixture()
	)
	wrongArrayCount[len(wrongArrayCount)-3] = 0x91
	wrongSampleType[len(wrongSampleType)-1] = 0xc0
	tests := []struct {
		name string
		wire []byte
		want error
	}{
		{name: "missing all children", wire: []byte{0x86}, want: io.ErrUnexpectedEOF},
		{name: "missing map value", wire: []byte{0x86, 0xa2, 'i', 'd'}, want: io.ErrUnexpectedEOF},
		{name: "wrong map count", wire: []byte{0x85}, want: errApplicationSchema},
		{name: "extra map field", wire: []byte{0x87}, want: errApplicationSchema},
		{name: "wrong root family", wire: []byte{0x96}, want: ErrTypeMismatch},
		{name: "unexpected key", wire: []byte{0x86, 0xa2, 'n', 'o', 0x07}, want: errApplicationSchema},
		{name: "reordered key", wire: []byte{0x86, 0xa4, 'n', 'a', 'm', 'e'}, want: errApplicationSchema},
		{name: "duplicate key", wire: []byte{0x86, 0xa2, 'i', 'd', 0x07, 0xa2, 'i', 'd'}, want: errApplicationSchema},
		{name: "wrong value family", wire: []byte{0x86, 0xa2, 'i', 'd', 0xc0}, want: ErrTypeMismatch},
		{name: "wrong nested count", wire: wrongArrayCount, want: errApplicationSchema},
		{name: "wrong nested value", wire: wrongSampleType, want: ErrTypeMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := decodeStreamingApplicationMessage(NewReader(bytes.NewReader(tt.wire)))
			if !errors.Is(err, tt.want) || !reflect.DeepEqual(message, applicationMessage{}) {
				t.Fatalf("invalid schema = (%+v, %v), want zero and %v", message, err, tt.want)
			}
		})
	}
}

func TestStreamNestedContainers(t *testing.T) {
	var (
		writer bytes.Buffer
		e      = NewWriter(&writer)
		steps  = []func() (err error){
			func() (err error) { return e.EncodeArrayHeader(2) },
			func() (err error) { return e.EncodeMapHeader(1) },
			func() (err error) { return e.EncodeString("k") },
			func() (err error) { return e.EncodeArrayHeader(2) },
			e.EncodeNil,
			func() (err error) { return e.EncodeMapHeader(0) },
			func() (err error) { return e.EncodeArrayHeader(0) },
			func() (err error) { return e.EncodeBool(true) },
		}
		wire = []byte{0x92, 0x81, 0xa1, 'k', 0x92, 0xc0, 0x80, 0x90, 0xc3}
	)
	for _, step := range steps {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(writer.Bytes(), wire) {
		t.Fatalf("nested container bytes = %x, want %x", writer.Bytes(), wire)
	}

	d := NewReader(bytes.NewReader(wire))
	if count, err := d.DecodeArrayHeader(); count != 2 || err != nil {
		t.Fatalf("outer array = (%d, %v)", count, err)
	}

	if count, err := d.DecodeMapHeader(); count != 1 || err != nil {
		t.Fatalf("nested map = (%d, %v)", count, err)
	}

	if key, err := d.DecodeString(); key != "k" || err != nil {
		t.Fatalf("nested key = (%q, %v)", key, err)
	}

	if count, err := d.DecodeArrayHeader(); count != 2 || err != nil {
		t.Fatalf("nested array = (%d, %v)", count, err)
	}

	if err := d.DecodeNil(); err != nil {
		t.Fatal(err)
	}

	if count, err := d.DecodeMapHeader(); count != 0 || err != nil {
		t.Fatalf("empty map = (%d, %v)", count, err)
	}

	if count, err := d.DecodeArrayHeader(); count != 0 || err != nil {
		t.Fatalf("empty array = (%d, %v)", count, err)
	}

	if value, err := d.DecodeBool(); !value || err != nil {
		t.Fatalf("following value = (%v, %v)", value, err)
	}
}

func TestStreamGeneratedArrayIsIncremental(t *testing.T) {
	const count uint32 = 100000
	var (
		writer = &streamArrayWriter{count: count}
		e      = NewWriter(writer)
		r      = newStreamArrayReader(count)
		d      = NewReader(r)
		total  uint64
	)
	if err := e.EncodeArrayHeader(count); err != nil {
		t.Fatal(err)
	}

	for value := uint64(0); value < uint64(count); value++ {
		if err := e.EncodeUint64(value); err != nil {
			t.Fatal(err)
		}

		if value == 0 && (writer.seen != 1 || writer.writes != 2) {
			t.Fatal("Writer delayed the first element until the array was complete")
		}
	}
	if writer.seen != uint64(count) || writer.maxRequest > 9 || e.state.buffer != nil {
		t.Fatalf("generated writes = %d values, max request %d, buffer %d", writer.seen, writer.maxRequest, len(e.state.buffer))
	}

	decodedCount, headerErr := d.DecodeArrayHeader()
	if headerErr != nil || decodedCount != count || r.next != 0 {
		t.Fatalf("generated array header = (%d, %v), generated elements %d", decodedCount, headerErr, r.next)
	}

	for index := uint64(0); index < uint64(decodedCount); index++ {
		value, readErr := d.DecodeUint64()
		if readErr != nil || value != index {
			t.Fatalf("generated element %d = (%d, %v)", index, value, readErr)
		}

		total += value
		if index == 0 && r.next != 1 {
			t.Fatal("Reader waited for ungenerated elements before returning the first")
		}
	}
	if total != uint64(count)*(uint64(count)-1)/2 || r.bytes != writer.bytes || r.maxRequest > 8 || d.state.buffer != nil {
		t.Fatalf("incremental total %d, wire bytes %d/%d, request %d, buffer %d", total, r.bytes, writer.bytes, r.maxRequest, len(d.state.buffer))
	}

	if err := d.DecodeNil(); !errors.Is(err, io.EOF) {
		t.Fatalf("generated array end = %v", err)
	}
}
