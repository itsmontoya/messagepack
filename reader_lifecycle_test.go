package messagepack

import (
	"bytes"
	"errors"
	"io"
	"math"
	"testing"
)

func TestReaderInitializationAndReset(t *testing.T) {
	var (
		zero  Reader
		tests = []struct {
			name string
			d    *Reader
		}{
			{name: "zero value", d: &zero},
			{name: "nil constructor", d: NewReader(nil)},
		}
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requireReaderScalarFailure(t, tt.d, ErrUninitialized)
			if tt.d.state != nil && tt.d.state.err != nil {
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
			requireReaderScalarFailure(t, tt.d, ErrUninitialized)
			tt.d.Reset(bytes.NewReader([]byte{0x07}))
			integer, integerErr := tt.d.DecodeUint64()
			if integer != 7 || integerErr != nil {
				t.Fatalf("Reset after failure = (%d, %v)", integer, integerErr)
			}
		})
	}
}

func TestReaderTypedNilReader(t *testing.T) {
	var reader *decoderTestReader
	d := NewReader(reader)
	value, err := d.DecodeBool()
	if !value || err != nil {
		t.Fatalf("typed nil reader was not called: (%v, %v)", value, err)
	}
}

func TestReaderLimits(t *testing.T) {
	var (
		defaults = ReaderLimits{MaxPayloadBytes: 16777216, MaxSkipDepth: 64,
			MaxSkipValues: 1048576, MaxSkipBytes: 67108864}
		custom = ReaderLimits{MaxPayloadBytes: 1, MaxSkipDepth: 256,
			MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64}
		zero Reader
	)
	zero.Reset(nil)
	if zero.state.limits != defaults || NewReader(nil).state.limits != defaults {
		t.Fatal("constructor/zero Reset did not initialize default limits")
	}
	var unattached Reader
	if err := unattached.SetLimits(custom); err != nil || unattached.state.limits != custom {
		t.Fatalf("unattached SetLimits = %v, limits %+v", err, unattached.state.limits)
	}
	unattached.Reset(bytes.NewReader([]byte{0xc3}))
	if unattached.state.limits != custom {
		t.Fatal("Reset discarded configured limits")
	}
	tests := []struct {
		name   string
		limits ReaderLimits
		want   ReaderLimits
	}{
		{name: "all defaults", want: defaults},
		{name: "small payload", limits: ReaderLimits{MaxPayloadBytes: 2},
			want: ReaderLimits{MaxPayloadBytes: 2, MaxSkipDepth: 64, MaxSkipValues: 1048576, MaxSkipBytes: 67108864}},
		{name: "wire and work maximum", limits: ReaderLimits{MaxPayloadBytes: 4294967295,
			MaxSkipDepth: 256, MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64},
			want: ReaderLimits{MaxPayloadBytes: 4294967295, MaxSkipDepth: 256,
				MaxSkipValues: math.MaxUint64, MaxSkipBytes: math.MaxUint64}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &decoderTestReader{data: []byte{0xc3}}
			d := NewReader(r)
			if err := d.SetLimits(tt.limits); err != nil || d.state.limits != tt.want || r.reads != 0 {
				t.Fatalf("SetLimits = %v, %+v, reads %d; want %+v", err, d.state.limits, r.reads, tt.want)
			}
			d.Reset(nil)
			if d.state.limits != tt.want {
				t.Fatal("nil Reset changed configured limits")
			}
		})
	}
	for _, invalid := range []ReaderLimits{{MaxPayloadBytes: 4294967296}, {MaxSkipDepth: 257}} {
		if err := unattached.SetLimits(invalid); err != ErrInvalidArgument || unattached.state.limits != custom {
			t.Fatalf("invalid limits = (%+v, %v), previous %+v", unattached.state.limits, err, custom)
		}
	}
}

func TestReaderSetLimitsPreservesFailure(t *testing.T) {
	var (
		r       = &decoderTestReader{data: []byte{0xc1, 0xc3}}
		d       = NewReader(r)
		failure = d.DecodeNil()
		reads   = r.reads
	)
	if failure != ErrInvalidFormat {
		t.Fatalf("initial failure = %v", failure)
	}
	if err := d.SetLimits(ReaderLimits{MaxPayloadBytes: 1}); err != nil {
		t.Fatal(err)
	}
	if err := d.SetLimits(ReaderLimits{MaxSkipDepth: 257}); err != ErrInvalidArgument {
		t.Fatalf("invalid SetLimits after failure = %v", err)
	}
	requireReaderScalarFailure(t, d, failure)
	if r.reads != reads || d.state.limits.MaxPayloadBytes != 1 {
		t.Fatal("SetLimits performed I/O, cleared failure, or partially applied invalid limits")
	}
	d.Reset(bytes.NewReader([]byte{0xc3}))
	value, err := d.DecodeBool()
	if !value || err != nil || d.state.limits.MaxPayloadBytes != 1 {
		t.Fatalf("Reset after SetLimits = (%v, %v), limits %+v", value, err, d.state.limits)
	}
}

func TestReaderCopiesShareEOFResetAndLimits(t *testing.T) {
	var (
		input      = &decoderTestReader{data: []byte{0xc3}, endErr: io.EOF}
		r          = NewReader(input)
		copied     = *r
		value, err = copied.DecodeBool()
		first      error
	)
	if err != nil || !value {
		t.Fatalf("copied DecodeBool = (%v, %v)", value, err)
	}
	first = r.DecodeNil()
	if !errors.Is(first, io.EOF) || input.reads != 1 {
		t.Fatalf("shared pending EOF = %v, reads %d", first, input.reads)
	}
	if err = copied.SetLimits(ReaderLimits{MaxPayloadBytes: 1}); err != nil {
		t.Fatal(err)
	}
	if err = copied.DecodeNil(); err != first || input.reads != 1 {
		t.Fatalf("copied terminal error = %v, reads %d", err, input.reads)
	}
	r.Reset(bytes.NewReader([]byte{0xa2, 'i', 'd'}))
	if _, err = copied.DecodeString(); err != ErrLimitExceeded {
		t.Fatalf("shared reset and limits = %v, want ErrLimitExceeded", err)
	}
	copied.Reset(bytes.NewReader([]byte{0xc0}))
	if err = r.DecodeNil(); err != nil || input.closes != 0 {
		t.Fatalf("DecodeNil after copied Reset = %v, closes %d", err, input.closes)
	}
}
