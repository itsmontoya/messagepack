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

## Reflection integration

The cached reflection plans now call Reader numeric batches for arrays and slices
of built-in int64, uint64, and float64 elements, including named collections and
struct fields. Value and pointer hooks retain priority. Named element types,
pointer/interface elements, narrower integer targets, float32 targets, and binary
byte slices keep their existing paths. Structs remain maps; no batch crosses map
keys or assumes declaration order.

The handlers read the normal array header and preserve length, visit, and depth
checks before children. Typed stack scratch holds at most 64 values (512 bytes),
independent of the advertised count. Completed prefixes update arrays; private
slice storage grows only from completed children and is published after complete
success. No child tag is peeked before a batch. Reader chooses vector eligibility
from actual encodings and already available bytes, without additional stream
read-ahead. The 64-value staging bound is not a SIMD threshold: pairs of wide
encodings and blocks of sixteen fixints remain the Reader's vector units.

### Decoder measurements

Measured November 5, 2026 on the same Go 1.27.2 / darwin/arm64 / Apple M3 environment.
Baseline executables were compiled at `d5cb0e8` with only the new
`reflection_numeric_benchmark_test.go` added; after-change executables used the
same benchmark source with the reflection integration. Both revisions were
compiled separately with and without `GOEXPERIMENT=simd`.

Each variant measures 168 workloads: seven encodings, sizes 1/2/16/1024, root
arrays, root slices, and structs containing a numeric slice, with both
bytes.Reader and an io.Reader wrapper returning one byte per Read. Wide integers
repeat 0x0123456789abcdef using d3/cf; float64 repeats 1.5 using cb; compact
integers repeat fixint 7. Mixed integers alternate fixint 7 and wide int64.
Widened floats repeat float32 1.5 using ca. Inputs and plans are prepared before
timing. Each timed iteration resets the bytes.Reader and calls Decoder.Decode
on reusable Decoder/target storage; slice replacement allocations remain timed.
Values and exact consumed bytes are checked after timing.

All four variants ran sequentially without concurrent tests or benchmarks,
using a writable GOCACHE in /private/tmp. Each workload has three 50 ms samples.
The commands for each revision were:

```sh
GOCACHE=/private/tmp/messagepack-reflection-cache go test -run '^$' -bench '^BenchmarkDecoderNumericCollections$' -benchmem -benchtime=50ms -count=3
GOCACHE=/private/tmp/messagepack-reflection-cache GOEXPERIMENT=simd go test -run '^$' -bench '^BenchmarkDecoderNumericCollections$' -benchmem -benchtime=50ms -count=3
```

The table gives median ns/op; speedups compare before and after within the same
build. Unless marked fragmented, inputs are bytes.Reader. Struct means a map
with one numeric slice field. The [complete CSV](reflection-numeric-benchmarks.csv)
records medians for every workload and all four variants, including allocations.

| Input / shape (1,024 elements) | Ordinary before | Ordinary after | Speedup | SIMD before | SIMD after | Speedup |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| Wide int64 / array | 28879 | 21511 | 1.34x | 28074 | 4691 | 5.98x |
| Wide int64 / slice | 62569 | 19129 | 3.27x | 63319 | 3102 | 20.41x |
| Wide int64 / struct | 63009 | 19167 | 3.29x | 62615 | 3130 | 20.00x |
| Wide uint64 / array | 26865 | 18401 | 1.46x | 26881 | 4699 | 5.72x |
| Wide uint64 / slice | 61877 | 16597 | 3.73x | 61994 | 3079 | 20.13x |
| Wide float64 / array | 24627 | 16491 | 1.49x | 24555 | 3945 | 6.22x |
| Wide float64 / slice | 59359 | 14999 | 3.96x | 60090 | 2816 | 21.34x |
| Compact int64 / array | 21431 | 13052 | 1.64x | 21495 | 3690 | 5.83x |
| Compact int64 / slice | 57392 | 11291 | 5.08x | 57998 | 2221 | 26.11x |
| Compact uint64 / slice | 57939 | 11584 | 5.00x | 58255 | 2196 | 26.53x |
| Mixed int64 / slice | 60342 | 16587 | 3.64x | 60070 | 15862 | 3.79x |
| Widened float32 / slice | 60379 | 15148 | 3.99x | 60974 | 15242 | 4.00x |
| Fragmented wide int64 / array | 62316 | 55663 | 1.12x | 61958 | 53785 | 1.15x |
| Fragmented wide int64 / slice | 97495 | 53834 | 1.81x | 97733 | 52516 | 1.86x |

Allocation results were identical between ordinary and SIMD builds. All measured
arrays remain at **0 B/op, 0 allocs/op**. Root slices and slice-containing structs
have these costs for all seven encodings and both input modes:

| Elements | Before B/op | Before allocs/op | After B/op | After allocs/op |
| ---: | ---: | ---: | ---: | ---: |
| 1 | 64 | 4 | 32 | 2 |
| 2 | 112 | 7 | 40 | 2 |
| 16 | 784 | 38 | 152 | 2 |
| 1024 | 58000 | 2061 | 24728 | 7 |

No measured workload increased allocated bytes or allocation counts. A few
singleton arrays were slower: the largest ordinary-build median regression was
fragmented wide int64, 99.8 to 102.9 ns/op (+3.1%); the largest SIMD-build
regression was in-memory wide float64, 63.88 to 64.76 ns/op (+1.4%). Three short
samples do not establish the significance of changes this small. The two-fixint
int64 array improved only 1.03x in the SIMD build (82.1 to 79.8 ns/op), versus
1.11x in the ordinary build (82.9 to 74.35 ns/op). The SIMD build's inspection
overhead still matters below a vector block, so no universal collection-size
threshold follows from these results.

Ordinary builds already gain from avoiding per-element reflection dispatch and
from typed slice growth. The SIMD build adds contiguous input access and the
existing vector kernels; the earlier kernel measurements show why these gains
cannot be attributed entirely to SIMD instructions. Alternating compact/wide
integers and widened float32 do not form supported vector prefixes here, and the
fragmented source is ineligible for the contiguous-input path. Their improvements
come from the reflection integration rather than vector execution.

### Validation

gofmt, go vet, full tests (including localhost HTTP listeners), and race tests
passed with and without GOEXPERIMENT=simd. Tests cover root/nested arrays and
slices, named collections/elements, value/pointer hooks, empty through multichunk
inputs, integer widths and conversions, float widening and NaN bits, errors and
terminal state, array prefixes and private slice publication, traversal budgets,
memory/buffered/fragmented sources, and concurrent recursive plan initialization.
Linux/amd64 SIMD and Linux/386 with GOEXPERIMENT=simd (unsupported-architecture
scalar fallback) test binaries cross-compiled successfully; they were not run.
