package messagepack

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

const streamFuzzInputLimit = 4096

var (
	errStreamFuzzSource      = errors.New("stream fuzz source failure")
	errStreamFuzzDestination = errors.New("stream fuzz destination failure")
)

// streamFuzzSeeds uses literal bytes independently of the append primitives.
func streamFuzzSeeds() (seeds [][]byte) {
	seeds = [][]byte{
		{}, {0xc0}, {0xc2}, {0xc3}, {0x00}, {0x7f}, {0xe0}, {0xff},
		{0xcc, 0x80}, {0xcc, 0xff}, {0xcd, 1, 0}, {0xcd, 0xff, 0xff},
		{0xce, 0, 1, 0, 0}, {0xce, 0xff, 0xff, 0xff, 0xff},
		{0xcf, 0, 0, 0, 1, 0, 0, 0, 0},
		{0xcf, 0x7f, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		{0xcf, 0x80, 0, 0, 0, 0, 0, 0, 0},
		{0xcf, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		{0xd0, 0x80}, {0xd0, 0xdf}, {0xd0, 1},
		{0xd1, 0x80, 0}, {0xd1, 0xff, 0x7f}, {0xd1, 0, 1},
		{0xd2, 0x80, 0, 0, 0}, {0xd2, 0xff, 0xff, 0x7f, 0xff},
		{0xd3, 0x80, 0, 0, 0, 0, 0, 0, 0},
		{0xca, 0, 0, 0, 0}, {0xca, 0x80, 0, 0, 0}, {0xca, 0, 0, 0, 1},
		{0xca, 0x7f, 0x80, 0, 0}, {0xca, 0xff, 0x80, 0, 0},
		{0xca, 0x7f, 0x81, 0x23, 0x45},
		{0xcb, 0, 0, 0, 0, 0, 0, 0, 0}, {0xcb, 0x80, 0, 0, 0, 0, 0, 0, 0},
		{0xcb, 0x7f, 0xf0, 0, 0, 0, 0, 0, 0}, {0xcb, 0xff, 0xf0, 0, 0, 0, 0, 0, 0},
		{0xcb, 0x7f, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc},
		{0xa0}, {0xa2, 0xff, 0}, {0xd9, 2, 0xff, 0},
		{0xda, 0, 2, 0xff, 0}, {0xdb, 0, 0, 0, 2, 0xff, 0},
		{0xbf}, {0xd9, 32}, {0xd9, 0xff}, {0xda, 1, 0}, {0xda, 0xff, 0xff},
		{0xdb, 0, 1, 0, 0}, {0xdb, 0x7f, 0xff, 0xff, 0xff},
		{0xdb, 0x80, 0, 0, 0}, {0xdb, 0xff, 0xff, 0xff, 0xff},
		{0xc4, 0}, {0xc4, 2, 0xff, 0}, {0xc5, 0, 2, 0xff, 0}, {0xc6, 0, 0, 0, 2, 0xff, 0},
		{0xc4, 0xff}, {0xc5, 1, 0}, {0xc5, 0xff, 0xff}, {0xc6, 0, 1, 0, 0},
		{0xc6, 0x7f, 0xff, 0xff, 0xff}, {0xc6, 0x80, 0, 0, 0}, {0xc6, 0xff, 0xff, 0xff, 0xff},
		{0x90}, {0x9f}, {0xdc, 0, 16}, {0xdc, 0xff, 0xff}, {0xdd, 0, 1, 0, 0},
		{0xdd, 0xff, 0xff, 0xff, 0xff}, {0x80}, {0x8f}, {0xde, 0, 16},
		{0xde, 0xff, 0xff}, {0xdf, 0, 1, 0, 0}, {0xdf, 0xff, 0xff, 0xff, 0xff},
		{0xdc, 0, 1, 0xc0}, {0xdd, 0, 0, 0, 1, 0xc3},
		{0xde, 0, 1, 0xc0, 0xc3}, {0xdf, 0, 0, 0, 1, 0xc0, 0xc3},
		{0x92, 0x91, 0x81, 0xc0, 0x90, 0x81, 0xc0, 0x91, 0xc3},
		{0x81, 0x91, 1, 0x81, 0xc0, 0xc3}, {0x91}, {0x81, 0xc0},
	}
	for _, tag := range []byte{0xc1, 0xc7, 0xc8, 0xc9, 0xd4, 0xd5, 0xd6, 0xd7, 0xd8} {
		seeds = append(seeds, []byte{tag}, []byte{0x91, tag})
	}
	return seeds
}

// streamFuzzPayloadHeader does not materialize a body or use Reader internals.
func streamFuzzPayloadHeader(src []byte, isString bool) (length uint64, header int, err error) {
	if len(src) == 0 {
		return 0, 0, ErrTruncated
	}
	tag := src[0]
	switch {
	case isString && tag >= 0xa0 && tag <= 0xbf:
		return uint64(tag & 31), 1, nil
	case isString && tag == 0xd9 || !isString && tag == 0xc4:
		header = 2
	case isString && tag == 0xda || !isString && tag == 0xc5:
		header = 3
	case isString && tag == 0xdb || !isString && tag == 0xc6:
		header = 5
	default:
		return 0, 0, readTypeError(tag)
	}
	if len(src) < header {
		return 0, 0, ErrTruncated
	}
	switch header {
	case 2:
		length = uint64(src[1])
	case 3:
		length = uint64(binary.BigEndian.Uint16(src[1:3]))
	case 5:
		length = uint64(binary.BigEndian.Uint32(src[1:5]))
	}
	return length, header, nil
}

func streamFuzzError(err error, inputLength int) (out error) {
	if err != ErrTruncated {
		return err
	}
	if inputLength == 0 {
		return io.EOF
	}
	return io.ErrUnexpectedEOF
}

func requireStreamFuzzError(t *testing.T, got, want error) {
	t.Helper()
	if !errors.Is(got, want) || errors.Is(got, ErrTruncated) {
		t.Fatalf("stream error = %v, want %v without slice ErrTruncated", got, want)
	}
	if want == io.ErrUnexpectedEOF && errors.Is(got, io.EOF) {
		t.Fatal("incomplete stream value also matches clean EOF")
	}
	if want == ErrInvalidFormat || want == ErrUnsupportedType || want == ErrTypeMismatch || want == ErrOverflow || want == ErrLimitExceeded {
		if got != want {
			t.Fatalf("codec sentinel was wrapped: %v", got)
		}
	}
}

func requireStreamFuzzTerminal(t *testing.T, d *Reader, r *streamFuzzReader, failure error) {
	t.Helper()
	var (
		reads  = r.reads
		writer streamFuzzWriter
		limits = d.state.limits
	)
	requireReaderPayloadFailure(t, d, failure)
	requireStreamHeaderFailure(t, d, failure)
	requireSkipFailure(t, d, failure)
	if n, err := d.DecodeStringTo(&writer); n != 0 || err != failure {
		t.Fatalf("terminal real-destination transfer = (%d, %v)", n, err)
	}
	if err := d.SetLimits(limits); err != nil {
		t.Fatal(err)
	}
	if err := d.SetLimits(ReaderLimits{MaxSkipDepth: 257}); err != ErrInvalidArgument {
		t.Fatal("invalid SetLimits did not preserve its direct sentinel")
	}
	if err := d.Skip(); err != failure || r.reads != reads || writer.writes != 0 {
		t.Fatal("terminal operation or SetLimits changed failure or performed I/O")
	}
	d.Reset(nil)
	if err := d.Skip(); err != ErrUninitialized || r.reads != reads || d.state.limits != limits {
		t.Fatal("nil Reset performed I/O or lost configured limits")
	}
	fresh := &streamFuzzReader{data: []byte{0xc3}, endErr: io.EOF, dataError: true}
	d.Reset(fresh)
	if fresh.reads != 0 || d.state.limits != limits {
		t.Fatal("Reset read or changed limits")
	}
	if value, err := d.DecodeBool(); !value || err != nil {
		t.Fatalf("Reset did not clear terminal state: (%v, %v)", value, err)
	}
	err := d.DecodeNil()
	if !errors.Is(err, io.EOF) || fresh.reads != 1 || r.reads != reads {
		t.Fatal("pending EOF performed I/O or Reset retained the old source")
	}
}

func requireStreamFuzzInput(t *testing.T, src, before []byte, r *streamFuzzReader, d *Reader) {
	t.Helper()
	if !bytes.Equal(src, before) || r.position < 0 || r.position > len(src) {
		t.Fatal("decoder mutated input or consumed outside the available prefix")
	}
	if len(d.state.buffer) > 32768 || r.reads > (len(src)+1)*101+102 {
		t.Fatal("decoder exceeded bounded scratch or scheduled read work")
	}
}
