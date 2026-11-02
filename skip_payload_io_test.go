package messagepack

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

func TestSkipLargePayloadPreflightAndFailures(t *testing.T) {
	const length uint64 = 32768 + 9
	var (
		failure = errors.New("late skip source failure")
		header  = []byte{0xc6, 0, 0, 0x80, 9}
		tests   = []struct {
			name       string
			byteBudget uint64
			available  uint64
			end        error
			want       error
			read       uint64
		}{
			{name: "declared payload exceeds budget", byteBudget: length + 4,
				available: length, want: ErrLimitExceeded},
			{name: "later chunk EOF", byteBudget: length + 5,
				available: length - 2, end: io.EOF, want: io.ErrUnexpectedEOF, read: length - 2},
			{name: "later chunk source error", byteBudget: length + 5,
				available: length - 2, end: failure, want: failure, read: length - 2},
		}
	)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				payload = &encoderGeneratedReader{length: tt.available}
				end     = &decoderTestReader{endErr: tt.end}
				d       = NewDecoder(io.MultiReader(bytes.NewReader(header), payload, end))
			)
			if err := d.SetLimits(DecoderLimits{MaxSkipBytes: tt.byteBudget}); err != nil {
				t.Fatal(err)
			}
			err := d.Skip()
			if !errors.Is(err, tt.want) || payload.read != tt.read || payload.maxRequest > payloadBufferSize {
				t.Fatalf("Skip() = %v, source bytes %d, max request %d", err, payload.read, payload.maxRequest)
			}
			if tt.want == ErrLimitExceeded && (d.buffer != nil || payload.reads != 0 || end.reads != 0) {
				t.Fatal("over-budget payload allocated scratch or accessed its source")
			}
			reads, endReads := payload.reads, end.reads
			requireSkipFailure(t, d, err)
			if payload.reads != reads || end.reads != endReads {
				t.Fatal("payload failure performed later reads")
			}
		})
	}
}
