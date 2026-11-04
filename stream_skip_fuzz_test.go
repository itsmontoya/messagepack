package messagepack

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func FuzzStreamSkip(f *testing.F) {
	for _, wire := range streamFuzzSeeds() {
		f.Add(wire, []byte{0, 0, 1, 1, 0xff, 0x0f, 0xff, 0x0f, 15})
		for length := 0; length < len(wire); length++ {
			f.Add(wire[:length], []byte{0, 0, 1, 1, 31, 0, 31, 0, 3})
		}
		f.Add(append(append([]byte(nil), wire...), 0xc3, 0xc1), []byte{99, 1, 2, 1, 31, 0, 31, 0, 3})
	}
	for _, depth := range []int{1, 2, 16, 17, 64, 256, 257} {
		f.Add(append(bytes.Repeat([]byte{0x91}, depth), 0x90), []byte{0, 0, 0, 0, 0xff, 0x0f, 0xff, 0x0f, 15})
	}
	f.Fuzz(func(t *testing.T, src, controls []byte) {
		if len(src) > streamFuzzInputLimit || len(controls) > 32 {
			t.Skip()
		}
		var (
			settings [9]byte
			before   = append([]byte(nil), src...)
		)
		copy(settings[:], controls)
		var (
			limits = ReaderLimits{MaxPayloadBytes: 1, MaxSkipDepth: uint32(settings[8])%16 + 1,
				MaxSkipBytes:  uint64(binary.LittleEndian.Uint16(settings[4:6]))%4096 + 1,
				MaxSkipValues: uint64(binary.LittleEndian.Uint16(settings[6:8]))%4096 + 1}
			plain       = bytes.NewReader(src)
			baseline    = NewReader(plain)
			values      uint32
			deepest     uint32
			baselineErr error
		)
		if err := baseline.SetLimits(limits); err != nil {
			t.Fatal(err)
		}
		baselineErr = baseline.Skip()
		consumed := len(src) - plain.Len()
		oracleSize, oracleErr := streamFuzzValueSize(src, 0, &values, &deepest)
		if baselineErr == nil && (oracleErr != nil || oracleSize != consumed) {
			t.Fatalf("Skip boundary %d disagrees with bounded slice walk (%d, %v)", consumed, oracleSize, oracleErr)
		}
		if oracleErr == nil {
			var want error
			if uint64(oracleSize) > limits.MaxSkipBytes || uint64(values) > limits.MaxSkipValues || deepest > limits.MaxSkipDepth {
				want = ErrLimitExceeded
			}
			requireStreamFuzzError(t, baselineErr, want)
		}
		r := &streamFuzzReader{data: src, fragments: controls, empty: int(settings[0]) % 102,
			emptyBetween: int(settings[1]) % 102, dataError: settings[3]&1 != 0}
		if settings[2]%3 == 1 {
			r.endErr = io.EOF
		}
		if settings[2]%3 == 2 {
			r.endErr = errStreamFuzzSource
		}
		d := NewReader(r)
		if err := d.SetLimits(limits); err != nil {
			t.Fatal(err)
		}
		var (
			err  = d.Skip()
			want = baselineErr
		)
		if errors.Is(want, io.EOF) {
			want = io.EOF
		}
		if errors.Is(want, io.ErrUnexpectedEOF) {
			want = io.ErrUnexpectedEOF
		}
		if r.noProgress {
			want = io.ErrNoProgress
		}
		if r.deliveredError == errStreamFuzzSource {
			want = errStreamFuzzSource
		}
		requireStreamFuzzError(t, err, want)
		requireStreamFuzzInput(t, src, before, r, d)
		if uint64(r.position) > limits.MaxSkipBytes || uint64(consumed) > limits.MaxSkipBytes {
			t.Fatal("Skip consumed above its wire-byte budget")
		}
		if !r.noProgress && r.deliveredError != errStreamFuzzSource && r.position != consumed {
			t.Fatalf("fragmented Skip consumed %d, contiguous Skip %d", r.position, consumed)
		}
		if err != nil {
			requireStreamFuzzTerminal(t, d, r, err)
			return
		}
		// Repeat the exact accepted root twice to verify per-call budgets and an
		// intact following sentinel, including nested map keys and last children.
		wire := append(append(append([]byte(nil), src[:consumed]...), src[:consumed]...), 0xc3)
		fresh := &streamFuzzReader{data: wire, fragments: controls, endErr: io.EOF, dataError: true}
		d.Reset(fresh)
		if d.Skip() != nil || fresh.position != consumed || d.Skip() != nil || fresh.position != 2*consumed {
			t.Fatal("Skip budgets did not restart or the next root boundary moved")
		}
		if value, nextErr := d.DecodeBool(); !value || nextErr != nil {
			t.Fatalf("following sentinel = (%v, %v)", value, nextErr)
		}
	})
}

// streamFuzzValueSize is a bounded, recursive slice walk, independent of Skip's
// iterative stack and outstanding-sibling accounting. It never builds a tree.
func streamFuzzValueSize(src []byte, depth uint32, values, deepest *uint32) (n int, err error) {
	if len(src) == 0 {
		return 0, ErrTruncated
	}
	if depth > 32 || *values == streamFuzzInputLimit {
		return 0, ErrLimitExceeded
	}
	*values += 1
	var (
		tag      = src[0]
		count    uint32
		children uint64
	)
	switch {
	case tag <= 0x7f || tag >= 0xe0 || tag == 0xc0 || tag == 0xc2 || tag == 0xc3:
		return 1, nil
	case tag == 0xcc || tag == 0xcd || tag == 0xce || tag == 0xcf:
		_, n, err = ReadUint64(src)
		return n, err
	case tag >= 0xd0 && tag <= 0xd3:
		_, n, err = ReadInt64(src)
		return n, err
	case tag == 0xca:
		_, n, err = ReadFloat32(src)
		return n, err
	case tag == 0xcb:
		_, n, err = ReadFloat64(src)
		return n, err
	case tag >= 0xa0 && tag <= 0xbf || tag >= 0xd9 && tag <= 0xdb:
		_, n, err = ReadStringBytes(src)
		return n, err
	case tag >= 0xc4 && tag <= 0xc6:
		_, n, err = ReadBytes(src)
		return n, err
	case tag >= 0x90 && tag <= 0x9f || tag == 0xdc || tag == 0xdd:
		count, n, err = ReadArrayHeader(src)
		children = uint64(count)
	case tag >= 0x80 && tag <= 0x8f || tag == 0xde || tag == 0xdf:
		count, n, err = ReadMapHeader(src)
		children = uint64(count) * 2
	default:
		return 0, readTypeError(tag)
	}
	if err != nil {
		return 0, err
	}
	depth++
	if depth > *deepest {
		*deepest = depth
	}
	if children > uint64(len(src)-n) {
		return 0, ErrTruncated
	}
	for child := uint64(0); child < children; child++ {
		var size int
		size, err = streamFuzzValueSize(src[n:], depth, values, deepest)
		if err != nil {
			return 0, err
		}
		n += size
	}
	return n, nil
}
