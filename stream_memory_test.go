package messagepack

import (
	"runtime"
	"testing"
)

func TestStreamTransferAllocationBytes(t *testing.T) {
	for _, operation := range []struct {
		name     string
		encode   bool
		isString bool
	}{
		{name: "EncodeBytes", encode: true},
		{name: "EncodeString", encode: true, isString: true},
		{name: "DecodeBytes"},
		{name: "DecodeString", isString: true},
	} {
		t.Run(operation.name, func(t *testing.T) {
			for _, reuse := range []bool{false, true} {
				name := "New"
				if reuse {
					name = "Reset"
				}
				t.Run(name, func(t *testing.T) {
					var (
						minimum = ^uint64(0)
						maximum uint64
					)
					for _, size := range streamBenchmarkSizes {
						allocated := measureStreamTransferAllocation(t, size.length, operation.encode, operation.isString, reuse)
						t.Logf("%s: %d B/transfer over 16 transfers", size.name, allocated)
						// Four copy buffers allow allocator/runtime noise without
						// permitting even one payload-sized allocation at 8 MiB.
						if allocated > 4*payloadBufferSize {
							t.Fatalf("%s allocated %d B/transfer, budget %d", size.name, allocated, 4*payloadBufferSize)
						}
						if allocated < minimum {
							minimum = allocated
						}
						if allocated > maximum {
							maximum = allocated
						}
					}
					// This is a scaling bound, not an exact allocator-size promise.
					if maximum-minimum > 2*payloadBufferSize {
						t.Fatalf("allocation spread %d B/transfer exceeds %d", maximum-minimum, 2*payloadBufferSize)
					}
				})
			}
		})
	}
}

func measureStreamTransferAllocation(t *testing.T, length uint64, encode bool, isString bool, reuse bool) (bytesPerRun uint64) {
	t.Helper()
	var (
		source  streamBenchmarkPayloadReader
		sink    streamBenchmarkWriter
		encoder = NewWriter(&sink)
		decoder = NewDecoder(&source)
		before  runtime.MemStats
		after   runtime.MemStats
		run     func() (n uint64, err error)
		n       uint64
		err     error
	)
	if encode {
		run = func() (n uint64, err error) { return encoder.EncodeBytesFrom(length, &source) }
		if isString {
			run = func() (n uint64, err error) { return encoder.EncodeStringFrom(length, &source) }
		}
	} else {
		run = func() (n uint64, err error) { return decoder.DecodeBytesTo(&sink) }
		if isString {
			run = func() (n uint64, err error) { return decoder.DecodeStringTo(&sink) }
		}
	}
	if reuse {
		source.Reset(length, isString, !encode)
		if n, err = run(); err != nil || n != length {
			t.Fatalf("warm transfer = %d, %v", n, err)
		}
	}
	runtime.GC()
	runtime.ReadMemStats(&before)
	for index := 0; index < 16; index++ {
		source.Reset(length, isString, !encode)
		if encode {
			if reuse {
				encoder.Reset(&sink)
			} else {
				encoder = NewWriter(&sink)
				streamBenchmarkWriterSink = encoder
			}
		} else {
			if reuse {
				decoder.Reset(&source)
			} else {
				decoder = NewDecoder(&source)
				streamBenchmarkDecoderSink = decoder
			}
		}
		if n, err = run(); err != nil || n != length {
			t.Fatalf("transfer = %d, %v", n, err)
		}
	}
	runtime.ReadMemStats(&after)
	runtime.KeepAlive(encoder)
	runtime.KeepAlive(decoder)
	return (after.TotalAlloc - before.TotalAlloc) / 16
}

func TestStreamRetainedWorkingMemory(t *testing.T) {
	const cohort = 32
	for _, isString := range []bool{false, true} {
		name := "Bytes"
		if isString {
			name = "String"
		}
		t.Run(name, func(t *testing.T) {
			for _, size := range streamBenchmarkSizes {
				t.Run(size.name, func(t *testing.T) {
					var (
						encoders = make([]*Writer, cohort)
						decoders = make([]*Decoder, cohort)
						source   streamBenchmarkPayloadReader
						sink     streamBenchmarkWriter
						before   runtime.MemStats
						after    runtime.MemStats
						scratch  uint64
						n        uint64
						err      error
					)
					runtime.GC()
					runtime.ReadMemStats(&before)
					for index := 0; index < cohort; index++ {
						encoders[index] = NewWriter(&sink)
						decoders[index] = NewDecoder(&source)
						source.Reset(size.length, isString, false)
						if isString {
							n, err = encoders[index].EncodeStringFrom(size.length, &source)
						} else {
							n, err = encoders[index].EncodeBytesFrom(size.length, &source)
						}
						if err != nil || n != size.length {
							t.Fatalf("encode = %d, %v", n, err)
						}
						source.Reset(size.length, isString, true)
						if isString {
							n, err = decoders[index].DecodeStringTo(&sink)
						} else {
							n, err = decoders[index].DecodeBytesTo(&sink)
						}
						if err != nil || n != size.length {
							t.Fatalf("decode = %d, %v", n, err)
						}
						encoders[index].Reset(nil)
						decoders[index].Reset(nil)
						if cap(encoders[index].state.buffer) > payloadBufferSize || cap(decoders[index].buffer) > payloadBufferSize {
							t.Fatal("codec scratch capacity grew beyond the transfer bound")
						}
						scratch += uint64(cap(encoders[index].state.buffer) + cap(decoders[index].buffer))
					}
					runtime.GC()
					runtime.ReadMemStats(&after)
					// Keep every codec and its reachable storage live through the second GC.
					runtime.KeepAlive(encoders)
					runtime.KeepAlive(decoders)
					delta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
					t.Logf("%d live Writer/Decoder pairs: scratch=%d B, post-GC heap delta=%d B (%d B/pair)", cohort, scratch, delta, delta/cohort)
					// Heap deltas include process/runtime noise. Allow two times the
					// bounded scratch plus 1 MiB; do not assert an exact live heap size.
					budget := int64(cohort*4*payloadBufferSize + 1048576)
					if delta > budget {
						t.Fatalf("retained heap delta %d exceeds broad %d-byte budget", delta, budget)
					}
				})
			}
		})
	}
}

func TestStreamTransferScratchBounds(t *testing.T) {
	var (
		source  streamBenchmarkPayloadReader
		sink    streamBenchmarkWriter
		encoder = NewWriter(&sink)
		decoder = NewDecoder(&source)
		n       uint64
		err     error
	)
	for _, size := range streamBenchmarkSizes {
		for _, isString := range []bool{false, true} {
			source.Reset(size.length, isString, false)
			encoder.Reset(&sink)
			if isString {
				n, err = encoder.EncodeStringFrom(size.length, &source)
			} else {
				n, err = encoder.EncodeBytesFrom(size.length, &source)
			}
			if err != nil || n != size.length {
				t.Fatalf("encode = %d, %v", n, err)
			}
			source.Reset(size.length, isString, true)
			decoder.Reset(&source)
			if isString {
				n, err = decoder.DecodeStringTo(&sink)
			} else {
				n, err = decoder.DecodeBytesTo(&sink)
			}
			if err != nil || n != size.length {
				t.Fatalf("decode = %d, %v", n, err)
			}
			if cap(encoder.state.buffer) > payloadBufferSize || cap(decoder.buffer) > payloadBufferSize {
				t.Fatal("unexpected retained scratch bounds")
			}
		}
	}
}
