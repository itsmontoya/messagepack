# Optional SIMD batch decoding

Measured on November 5, 2026 with Go 1.27.2, darwin/arm64, Apple M3, and eight
logical CPUs. The implementation uses standard-library `simd/archsimd` intrinsics
on arm64 (NEON) and amd64 (runtime AVX check). Other builds use scalar decoding.
AMD64 and Linux/386 fallback tests cross-compiled; they were not executed here.

## Implementation and boundaries

The vector path handles pairs of nine-byte int64/uint64/float64 encodings and
blocks of sixteen positive fixints. Signed/unsigned conversion checks precede
writes; float bits are preserved. Other widths, negative fixints, float32 widening,
cached tags, pending EOF, and unhandled suffixes use the existing scalar methods.

The path inspects bytes.Reader, bytes.Buffer, or already-buffered bytes from a
caller-supplied bufio.Reader, then advances only past complete validated values.
It never fills a stream buffer. bytes.Reader uses bounded 144-byte stack scratch
for non-consuming inspection. Arbitrary io.Reader implementations retain scalar
I/O and error behavior. Arm64 binary inspection confirmed NEON VTBL instructions.

## End-to-end measurements

The two builds use identical benchmark source, prepared inputs, and reusable
Reader/destination storage. Each timed iteration resets a bytes.Reader, decodes
the requested count, and checks errors/counts. Values and consumed bytes are
validated outside timing. Wide inputs repeat the bit pattern 0x0123456789abcdef
with the corresponding nine-byte tag. Positive inputs repeat the fixint 7.
No headers, reflection, files, or network I/O are measured.

```sh
go test -run '^$' -bench '^BenchmarkReaderWideNumbers$/(Int64|Uint64|Float64|PositiveInt64|PositiveUint64)$/Elements(2|16|1024)$/(Scalar|Batch)$' -benchmem -benchtime=100ms -count=3
GOEXPERIMENT=simd go test -run '^$' -bench '^BenchmarkReaderWideNumbers$/(Int64|Uint64|Float64|PositiveInt64|PositiveUint64)$/Elements(2|16|1024)$/(Scalar|Batch)$' -benchmem -benchtime=100ms -count=3
```

Measurements used a writable GOCACHE in /private/tmp and ran sequentially with
no other tests or benchmarks active. Each build completed 15 per-value scalar
controls and 15 batch cases on the same inputs, with three samples each.
All samples reported **0 B/op and 0 allocs/op**.

The table shows median ns/op for complete batch-method calls. Ratios compare
the default scalar batch implementation with the optional optimized build.

| Input | Elements | Scalar batch ns/op | SIMD build batch ns/op | Ratio |
| --- | ---: | ---: | ---: | ---: |
| Int64 | 2 | 37.32 | 12.43 | 3.00x |
| Int64 | 16 | 275.9 | 26.47 | 10.42x |
| Int64 | 1024 | 17341 | 1332 | 13.02x |
| Uint64 | 2 | 32.91 | 12.39 | 2.66x |
| Uint64 | 16 | 242.2 | 26.02 | 9.31x |
| Uint64 | 1024 | 14876 | 1356 | 10.97x |
| Float64 | 2 | 30.17 | 11.31 | 2.67x |
| Float64 | 16 | 216.6 | 20.8 | 10.41x |
| Float64 | 1024 | 13353 | 1008 | 13.25x |
| PositiveInt64 | 2 | 22.75 | 30.16 | 0.75x |
| PositiveInt64 | 16 | 159.4 | 16.02 | 9.95x |
| PositiveInt64 | 1024 | 9681 | 382.3 | 25.32x |
| PositiveUint64 | 2 | 23.51 | 30.03 | 0.78x |
| PositiveUint64 | 16 | 165 | 15.2 | 10.86x |
| PositiveUint64 | 1024 | 10040 | 387 | 25.94x |

The wide-number cases improve even for two values. Two positive fixints fall
below the vector block size and pay inspection overhead before scalar fallback;
that case is slower with the experiment enabled. These short samples describe
this CPU and these inputs, not a general SIMD threshold or throughput guarantee.

## Conversion kernels

These checks remove Reader calls and memory-source inspection. Pair converts
1,024 already-identified wide values, two per iteration, without tag validation
in either implementation. Fixint validates and expands 1,024 one-byte values;
the scalar path validates each tag and the vector path validates blocks of 16.

```sh
GOEXPERIMENT=simd go test -run '^$' -bench '^BenchmarkNumeric(Pair|Fixint)Conversion$' -benchmem -benchtime=200ms -count=3
```

Times are median ns/op for 1,024 values, with zero heap allocations in every sample.

| Kernel | Scalar ns/op | SIMD ns/op |
| --- | ---: | ---: |
| Pair | 275.2 | 436.7 |
| Fixint | 266.4 | 272 |

Scalar byte reversal is faster than the wide SIMD shuffle on this M3; fixint
validation/expansion is close. The large end-to-end gains therefore come mainly
from processing contiguous input without per-element Reader dispatch. They must
not be attributed entirely to SIMD instructions. Future work should compare a
scalar contiguous-input implementation against these vector kernels on each CPU.
