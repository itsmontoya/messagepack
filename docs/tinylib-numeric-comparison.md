# Tinylib comparison after numeric reflection batching

Measured November 5, 2026 on Go 1.27.2, darwin/arm64, Apple M3, with eight
logical CPUs. Repository revision: `8b2505c` (implementation `f041aaf`). The
comparison pins tinylib/msgp v1.6.5, matching the earlier
[struct comparison](decoder-string-optimizations.md).

Our SIMD build is ahead on large positive-fixint and float64 slices, and near
parity on large wide-integer slices. Tinylib remains ahead on wide integer arrays,
small collections, mixed numeric encodings, float32 widening, and general small
structs. Ordinary builds remain behind on every measured numeric workload.
Our fresh unbuffered encoders use less memory and CPU time than tinylib's fresh
buffered encoders; matching their buffering reverses the CPU result.

## Numeric decoding

These are median ns/op for reused targets with 1,024 elements. Each build calls
our Decoder.Decode; tinylib calls its generated DecodeMsg. Struct means a map
containing one numeric slice field.

| Encoding / target | Ours ordinary | Ours SIMD | Tinylib |
| --- | ---: | ---: | ---: |
| Wide int64 / array | 20329 | 4716 | 2997 |
| Wide int64 / slice | 18435 | 2990 | 3117 |
| Wide int64 / struct | 18567 | 3079 | 3124 |
| Wide uint64 / array | 18719 | 4678 | 2908 |
| Wide uint64 / slice | 16936 | 2992 | 3029 |
| Wide uint64 / struct | 16928 | 3128 | 3030 |
| Wide float64 / array | 16556 | 3981 | 4774 |
| Wide float64 / slice | 15014 | 2689 | 4769 |
| Wide float64 / struct | 14992 | 2706 | 4787 |
| Positive fixint → int64 / array | 13070 | 3704 | 3836 |
| Positive fixint → int64 / slice | 11333 | 2124 | 3834 |
| Positive fixint → uint64 / slice | 11557 | 2095 | 3834 |
| Mixed compact/wide int64 / slice | 15823 | 16261 | 3756 |
| Float32 → float64 / slice | 15124 | 15200 | 6641 |

The SIMD build beats tinylib by 1.77–1.83x on the large float64 and positive-fixint
slices, including the equivalent struct-field cases. Large wide-integer slices
are within about 4% either way, so they are better described as near parity.
Wide int64/uint64 arrays still take 1.57–1.61x tinylib's time. Float64 arrays are
1.20x faster than tinylib here, while compact integer arrays are near parity.
Mixed integer slices take 4.33x tinylib's time; float32 widening takes 2.29x.
Those encodings do not form supported vector prefixes in this workload.

Short collections still favor tinylib. For two wide int64 values, arrays take
86.6 / 61.63 / 18.21 ns and slices take 109.8 / 84.35 / 18.45 ns for ordinary /
SIMD / tinylib. At sixteen values, wide int64 slices take 358.9 / 111.8 / 65.63 ns.
Enabling SIMD helps those wide values but does not remove the small-call gap.
Two compact int64 values are slower with SIMD enabled (80.04 versus 74.37 ns for
arrays); tinylib takes 22.04 ns. This is not a universal Go-type or count threshold.

All measured arrays allocate zero bytes in both libraries. Numeric slices and
slice-containing structs have the following allocation costs, independent of
ordinary/SIMD builds and the seven encodings:

| Destination | Ours B/op / allocs | Tinylib B/op / allocs |
| --- | ---: | ---: |
| Reused, 2 elements | 40 / 2 | 0 / 0 |
| Reused, 16 elements | 152 / 2 | 0 / 0 |
| Reused, 1024 elements | 24728 / 7 | 0 / 0 |
| Cleared, 1024 elements | 24728 / 7 | 8192 / 1 |

Clearing the target reduces tinylib's storage-reuse advantage but does not erase
the allocation difference. For example, cleared positive-fixint uint64 slices
take 2,095 ns with our SIMD build and 4,172 ns with tinylib; cleared float64 slices
take 2,642 and 5,109 ns, respectively. Throughput wins therefore coexist with
higher allocation costs. The publication-contract difference is described below.

## General encoding and decoding

Numeric batching does not close the general struct gap. These are median ns/op
for reused codecs; decode retains field values/capacity between operations.
Ordinary-build figures are shown because the SIMD build is similar on these
fixtures, whose numeric values are separated by map keys or use short scalar
encodings.

| Fixture | Our encode | Tinylib encode | Our decode | Tinylib decode |
| --- | ---: | ---: | ---: | ---: |
| Scalars | 71.6 | 20.19 | 229.2 | 54.05 |
| Mixed | 130.2 | 41.82 | 394.2 | 98.66 |
| Nested | 67.19 | 16.36 | 217.9 | 52.48 |
| String1KiB | 142.3 | 53.34 | 462.6 | 176.2 |
| Binary1KiB | 131.3 | 54.53 | 401.7 | 115.0 |
| Binary64KiB | 134.5 | 40.9 | 1150 | 937.9 |

Small scalar/mixed/nested decoding remains about 4.0–4.2x slower, with allocation
parity: zero allocations for scalar/nested fixtures, one 3-byte owned-string
allocation for reused Mixed, and one 1,024-byte allocation for String1KiB.
Cleared targets also retain allocation parity on these general fixtures. Binary
64 KiB decoding is closer at 1.23x tinylib's time, or 1.13x with cleared targets.

Caller buffering does not close the small-struct gap: buffered Mixed decode takes
421.7 ns versus native 394.2 ns and tinylib 98.66 ns. Actual output copying also
favors tinylib: Mixed encoding into bytes.Buffer takes 166.0 versus 43.67 ns;
Binary64KiB takes 953.2 versus 787.2 ns. Those copy cases allocate zero bytes in
both libraries. The much lower counting-sink Binary64KiB encode times above do
not include a full payload copy and should not be read as output-copy throughput.

Fresh-codec results show where avoiding a per-codec 4 KiB I/O buffer helps.
Type metadata is already warm in every case:

| Fresh operation | Our native ns/op | Our buffered ns/op | Tinylib ns/op | Our native B/op / allocs | Tinylib B/op / allocs |
| --- | ---: | ---: | ---: | ---: | ---: |
| Mixed encode | 146.5 | 426.1 | 340.5 | 96 / 2 | 4144 / 2 |
| String1KiB encode | 224.3 | 514.3 | 338.6 | 1120 / 3 | 4144 / 2 |
| Mixed decode | 427.4 | 680.9 | 376.7 | 150 / 4 | 4262 / 5 |
| String1KiB decode | 563.2 | 839.5 | 485.7 | 2194 / 5 | 5282 / 5 |

Our native fresh encoding is 2.32x faster for Mixed and 1.51x for String1KiB.
Tinylib is faster when our fresh encoders also allocate matching 4 KiB buffering.
Fresh decoding still favors tinylib on CPU while our native codec allocates fewer
bytes. These are codec-lifetime and buffering results, not evidence of faster
general reflection traversal.

## Method

Our Encoder.Encode and Decoder.Decode use cached reflection. Tinylib uses its
generated streaming EncodeMsg/DecodeMsg methods and a 4 KiB Reader/Writer.
Both libraries receive identical schemas, numeric wire bytes, and valid values.
Our native codec is unbuffered; separate general-struct controls add matching
caller-owned 4 KiB buffering. EncodeCopy writes actual bytes into a reusable
bytes.Buffer; counting-sink encoding results alone do not measure payload copying.

The ordinary and GOEXPERIMENT=simd builds use identical benchmark source.
Tinylib's ordinary build is the fixed generated-code control. Numeric cases use
bytes.Reader, allowing our optional contiguous-input path. This compares complete
streaming codec calls; it does not isolate SIMD instructions. The differences
also include reflection dispatch, input access, destination updates, and storage
management.

Numeric inputs cover wide int64/uint64/float64, compact positive fixints, alternating
compact/wide int64, and float32 widened to float64. Each has 2, 16, and 1,024
elements in a root array, root slice, or struct containing a slice. Root collections
are named Go types supported by each codec. At 1,024 elements, slices and struct
slices are measured both with reused destinations and with targets cleared inside
timing. Arrays use reusable storage.

General fixtures cover four scalar fields, a mixed six-field struct, nested
structs, 1 KiB strings, and 1 KiB / 64 KiB binary fields. Fresh-codec cases include
construction and result allocation with warmed type metadata. Reused-codec cases
exclude construction and warm-up. No cold type-plan construction, socket/file
latency, malformed-input throughput, or non-arm64 performance is measured.

Successful numeric slice semantics differ: tinylib reslices existing capacity or
allocates the advertised length before reading children. Our decoder builds
private replacement storage from completed children and publishes only on full
success. Tinylib therefore has an allocation advantage on reused slices; these
measurements do not make those failure/publication contracts equivalent.

Each case has five unprofiled 100 ms samples. Library/build groups run sequentially,
rotate, and reverse between rounds. Tests, compilation, and other benchmarks do
not run during sampling. Comparison tests verify wire equality for the general
fixtures, decoded values, ownership, and consumed bytes. Numeric tests also verify
that neither source bytes nor tinylib's internal buffer retain unread data.

The run completed **339 cases / 1,695 samples**: 119 cases for each of our two
builds and 101 for tinylib. Each library/build has 77 numeric cases; our additional
general controls cover caller buffering. The [complete CSV](tinylib-numeric-comparison.csv)
contains medians, sample ranges, allocated bytes, and allocation counts. These
local measurements have no statistical-significance or cross-platform claim;
near ties should not be treated as established wins.

Both builds passed the comparison-module tests and every benchmark's one-iteration
smoke run. Production code and the repository module dependencies were unchanged
by this comparison.

Likely next areas to investigate are scalar contiguous-input decoding, fixed-array
publication, and small-struct key handling. Arrays currently apply per-element
reflection setters while slices append typed chunks. This run did not collect CPU
profiles, so these are candidates for measurement rather than demonstrated causes
of the full remaining gaps.

## Reproduction

The isolated module and its generated fixtures, runner, executables, raw samples,
and source hashes are retained at:

```text
/private/tmp/messagepack-tinylib-batch-20261105
```

It uses a local replace directive for this checkout. Tinylib and its dependencies
are not added to the repository module. From the comparison directory:

```sh
export GOCACHE=/private/tmp/messagepack-go-build
export GOMODCACHE=/private/tmp/messagepack-library-comparison.jVX5vg/modcache
export GOPROXY=off
go test ./...
GOEXPERIMENT=simd go test ./...
go test -c -o ordinary.test
GOEXPERIMENT=simd go test -c -o simd.test
python3 run_batch.py
```

Numeric fixtures were generated with the pinned tinylib tool, for example:

```sh
go run github.com/tinylib/msgp -file int64_array_1024.go -o int64_array_1024_gen.go -tests=false
```

The comparison remains external to the package; no production generator or
dependency is introduced.
