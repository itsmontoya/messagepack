package messagepack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
)

func TestFlexibleStreamingApplicationMessage(t *testing.T) {
	for _, fixture := range [][]byte{applicationMessageFixture(), flexibleStreamingApplicationFixture()} {
		for _, fragment := range []int{0, 1, 3} {
			t.Run(fmt.Sprintf("fields_%d/fragment_%d", fixture[0]&0x0f, fragment), func(t *testing.T) {
				var (
					wire = append(append([]byte(nil), fixture...), 0xc3)
					r    = &decoderTestReader{data: wire, maxRead: fragment}
					d    = NewReader(r)
					want = applicationMessage{ID: 7, Name: "Ada", Active: true, Score: 1.5,
						Payload: []byte{1, 2}, Samples: [2]int64{-1, 2}}
				)

				message, err := decodeFlexibleStreamingApplicationMessage(d)
				if err != nil || !reflect.DeepEqual(message, want) || r.position != len(fixture) {
					t.Fatalf("flexible schema = (%+v, %v), position %d", message, err, r.position)
				}

				if value, nextErr := d.DecodeBool(); nextErr != nil || !value {
					t.Fatalf("following value = (%v, %v)", value, nextErr)
				}
			})
		}
	}
	_, strictErr := decodeStreamingApplicationMessage(NewReader(bytes.NewReader(flexibleStreamingApplicationFixture())))
	if strictErr != errApplicationSchema {
		t.Fatalf("strict schema changed its policy: %v", strictErr)
	}
}

func TestFlexibleStreamingApplicationInvalidSchema(t *testing.T) {
	var (
		missingID = flexibleStreamingApplicationFixture()
		duplicate = applicationMessageFixture()
	)

	missingID = missingID[:len(missingID)-4]
	missingID[0] = 0x86
	duplicate[0] = 0x87
	duplicate = append(duplicate, 0xa2, 'i', 'd', 8)
	tests := []struct {
		name string
		wire []byte
		want error
	}{
		{name: "required field omitted", wire: missingID, want: errApplicationSchema},
		{name: "duplicate required field", wire: duplicate, want: errApplicationSchema},
		{name: "duplicate unknown name", wire: []byte{0x88, 0xa1, 'x', 0xc0, 0xa1, 'x'}, want: errApplicationSchema},
		{name: "too few fields", wire: []byte{0x85}, want: errApplicationSchema},
		{name: "example count above sixteen", wire: []byte{0xde, 0, 17}, want: errApplicationSchema},
		{name: "huge count exceeds example policy", wire: []byte{0xdf, 0xff, 0xff, 0xff, 0xff}, want: errApplicationSchema},
		{name: "unsupported unknown value", wire: []byte{0x87, 0xa1, 'x', 0xd4}, want: ErrUnsupportedType},
		{name: "invalid unknown value", wire: []byte{0x87, 0xa1, 'x', 0xc1}, want: ErrInvalidFormat},
		{name: "truncated unknown container", wire: []byte{0x87, 0xa1, 'x', 0x81, 0xc0}, want: io.ErrUnexpectedEOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := decodeFlexibleStreamingApplicationMessage(NewReader(bytes.NewReader(tt.wire)))
			if !errors.Is(err, tt.want) || !reflect.DeepEqual(message, applicationMessage{}) {
				t.Fatalf("invalid flexible schema = (%+v, %v), want zero and %v", message, err, tt.want)
			}
		})
	}
}

func TestFlexibleStreamingApplicationSixteenFieldLimit(t *testing.T) {
	var (
		fixture = applicationMessageFixture()
		wire    = append([]byte{0xde, 0, 16}, fixture[1:]...)
	)

	for index := byte(0); index < 10; index++ {
		wire = append(wire, 0xa2, 'x', '0'+index, 0xc0)
	}
	message, err := decodeFlexibleStreamingApplicationMessage(NewReader(bytes.NewReader(wire)))
	if err != nil || message.ID != 7 || message.Name != "Ada" {
		t.Fatalf("sixteen-field schema = (%+v, %v)", message, err)
	}
}

func TestFlexibleStreamingApplicationTruncation(t *testing.T) {
	fixture := flexibleStreamingApplicationFixture()
	for length := 0; length < len(fixture); length++ {
		t.Run(fmt.Sprintf("length_%d", length), func(t *testing.T) {
			var (
				want = io.ErrUnexpectedEOF
				d    = NewReader(bytes.NewReader(fixture[:length]))
			)

			if length == 0 {
				want = io.EOF
			}

			message, err := decodeFlexibleStreamingApplicationMessage(d)
			if !errors.Is(err, want) || !reflect.DeepEqual(message, applicationMessage{}) {
				t.Fatalf("truncated flexible schema = (%+v, %v), want zero and %v", message, err, want)
			}

			if length > 0 && errors.Is(err, io.EOF) {
				t.Fatal("missing child became a clean sequence end")
			}
		})
	}
}
