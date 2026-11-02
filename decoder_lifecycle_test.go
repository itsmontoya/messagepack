package messagepack

import (
	"bytes"
	"errors"
	"io"
	"math"
	"testing"
)

func TestDecoderInitializationAndReset(t *testing.T) {
	var (
		zero  Decoder
		tests = []struct {
			name string
			d    *Decoder
		}{
			{name: "zero value", d: &zero},
			{name: "nil constructor", d: NewDecoder(nil)},
		}
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireDecoderScalarFailure(t, tt.d, ErrUninitialized)
			if tt.d.err != nil {
				t.Fatal("uninitialized error became terminal")
			}
			r := &decoderTestReader{data: []byte{0xc3}, endErr: io.EOF}
			tt.d.Reset(r)
			if r.reads != 0 {
				t.Fatal("Reset read input")
			}
			value, err := tt.d.DecodeBool()
			if !value || err != nil {
				t.Fatalf("first attached decode = (%v, %v)", value, err)
			}
			tt.d.Reset(bytes.NewReader([]byte{0xc0}))
			if err = tt.d.DecodeNil(); err != nil {
				t.Fatalf("Reset did not clear pending EOF: %v", err)
			}
			if r.closes != 0 || r.reads != 1 {
				t.Fatal("Reset closed or drained previous reader")
			}
			err = tt.d.DecodeNil()
			if !errors.Is(err, io.EOF) {
				t.Fatalf("expected terminal EOF, got %v", err)
			}
			tt.d.Reset(nil)
			requireDecoderScalarFailure(t, tt.d, ErrUninitialized)
			tt.d.Reset(bytes.NewReader([]byte{0x07}))
			integer, integerErr := tt.d.DecodeUint64()
			if integer != 7 || integerErr != nil {
				t.Fatalf("Reset after failure = (%d, %v)", integer, integerErr)
			}
		})
	}
}

func TestDecoderTypedNilReader(t *testing.T) {
	var reader *decoderTestReader
	d := NewDecoder(reader)
	value, err := d.DecodeBool()
	if !value || err != nil {
		t.Fatalf("typed nil reader was not called: (%v, %v)", value, err)
	}
}

func TestDecoderLimits(t *testing.T) {
	var (
		defaults = DecoderLimits{MaxPayloadBytes: 16777216, MaxSkipDepth: 64,
			MaxSkipValues: 1048576, MaxSkipBytes: 67108864}
		custom = DecoderLimits{MaxPayloadBytes: 1, MaxSkipDepth: 256,
			MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64}
		zero Decoder
	)
	zero.Reset(nil)
	if zero.limits != defaults || NewDecoder(nil).limits != defaults {
		t.Fatal("constructor/zero Reset did not initialize default limits")
	}
	var unattached Decoder
	if err := unattached.SetLimits(custom); err != nil || unattached.limits != custom {
		t.Fatalf("unattached SetLimits = %v, limits %+v", err, unattached.limits)
	}
	unattached.Reset(bytes.NewReader([]byte{0xc3}))
	if unattached.limits != custom {
		t.Fatal("Reset discarded configured limits")
	}
	tests := []struct {
		name   string
		limits DecoderLimits
		want   DecoderLimits
	}{
		{name: "all defaults", want: defaults},
		{name: "small payload", limits: DecoderLimits{MaxPayloadBytes: 2},
			want: DecoderLimits{MaxPayloadBytes: 2, MaxSkipDepth: 64, MaxSkipValues: 1048576, MaxSkipBytes: 67108864}},
		{name: "wire and work maximum", limits: DecoderLimits{MaxPayloadBytes: 4294967295,
			MaxSkipDepth: 256, MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64},
			want: DecoderLimits{MaxPayloadBytes: 4294967295, MaxSkipDepth: 256,
				MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &decoderTestReader{data: []byte{0xc3}}
			d := NewDecoder(r)
			if err := d.SetLimits(tt.limits); err != nil || d.limits != tt.want || r.reads != 0 {
				t.Fatalf("SetLimits = %v, %+v, reads %d; want %+v", err, d.limits, r.reads, tt.want)
			}
			d.Reset(nil)
			if d.limits != tt.want {
				t.Fatal("nil Reset changed configured limits")
			}
		})
	}
	for _, invalid := range []DecoderLimits{{MaxPayloadBytes: 4294967296}, {MaxSkipDepth: 257}} {
		if err := unattached.SetLimits(invalid); err != ErrInvalidArgument || unattached.limits != custom {
			t.Fatalf("invalid limits = (%+v, %v), previous %+v", unattached.limits, err, custom)
		}
	}
}

func TestDecoderSetLimitsPreservesFailure(t *testing.T) {
	var (
		r       = &decoderTestReader{data: []byte{0xc1, 0xc3}}
		d       = NewDecoder(r)
		failure = d.DecodeNil()
		reads   = r.reads
	)
	if failure != ErrInvalidFormat {
		t.Fatalf("initial failure = %v", failure)
	}
	if err := d.SetLimits(DecoderLimits{MaxPayloadBytes: 1}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetLimits(DecoderLimits{MaxSkipDepth: 257}); err != ErrInvalidArgument {
		t.Fatalf("invalid SetLimits after failure = %v", err)
	}
	requireDecoderScalarFailure(t, d, failure)
	if r.reads != reads || d.limits.MaxPayloadBytes != 1 {
		t.Fatal("SetLimits performed I/O, cleared failure, or partially applied invalid limits")
	}
	d.Reset(bytes.NewReader([]byte{0xc3}))
	value, err := d.DecodeBool()
	if !value || err != nil || d.limits.MaxPayloadBytes != 1 {
		t.Fatalf("Reset after SetLimits = (%v, %v), limits %+v", value, err, d.limits)
	}
}
