# Decoder and string optimizations

Measured on 2026-11-05 with Go 1.27.2, darwin/arm64, Apple M3, and the default eight logical CPUs. The baseline is commit `4dcd24df47df3a282b75bdd0fd7ba2b2a92d1180`; the updated executable contains the changes described below. Both use identical benchmark source. Tinylib uses generated streaming codecs from [msgp v1.6.5](https://github.com/tinylib/msgp/releases/tag/v1.6.5).

Warmed small-struct decoding takes 21–24% less time, and the 1 KiB string case takes 26% less time. The measured reused decoders now match tinylib’s allocation counts and byte counts. Fresh unbuffered 1 KiB string encoding is 7.18 times faster and allocates 96.6% fewer bytes. Warmed encoding timings remain within a few percent of the baseline.

## Implementation

- Decoder owns its reflected traversal context and Reader scratch. Each reflected root resets the visit budget and started flag. Scalar and reused binary-field decoding no longer allocate the 48-byte per-call context.
- Reflection plans link pointer targets, struct fields, array/slice elements, and map keys/values directly to child decoder plans. Child initialization stays lazy for recursive types. Dynamic interface values still resolve their runtime types. Struct name lookup indexes the cached exported-field metadata, preserving original Go field positions.
- Owned string reads longer than the nine-byte scalar scratch, through 32 KiB, reuse bounded Reader staging storage. DecodeString still makes an owned string copy. Its small wrapper remains inlineable, preserving temporary field-key lookup behavior.
- In-memory string writes grow reusable scratch to the payload size, capped at 32 KiB. Payload transfers and Skip retain their existing bounded transfer buffers. Ordinary writes remain immediate and include the existing short-write and stored-error handling.

String staging reuse applies after storage is warm and sufficient. The first read or growth still allocates staging, and fresh decoder allocation totals in the measured fixtures are unchanged. Strings larger than 32 KiB retain the existing temporary payload-sized slice plus owned-string allocation. Reader/Writer copies share their buffer state; returned strings remain independent of it.

## Reused decoding

Times are medians in ns/op. These cases retain destination fields and byte-slice capacity. Our native Encoder/Decoder is unbuffered; tinylib buffers internally at 4 KiB. Each operation resets its input to the same valid message.

| Fixture | Before | After | Time reduction | Tinylib | After / tinylib |
| --- | ---: | ---: | ---: | ---: | ---: |
| Scalars | 316 | 248.7 | 21.3% | 53.88 | 4.62× |
| Mixed | 560.1 | 440.6 | 21.3% | 98.12 | 4.49× |
| Nested | 319.1 | 243.5 | 23.7% | 52.49 | 4.64× |
| Binary1KiB | 568.1 | 448.1 | 21.1% | 115.3 | 3.89× |
| Binary64KiB | 1,264 | 1,225 | 3.1% | 959.8 | 1.28× |
| String1KiB | 695.2 | 513.5 | 26.1% | 176.8 | 2.90× |

Allocated bytes and allocation counts per operation:

| Fixture | Before B/op / allocs | After B/op / allocs | Tinylib B/op / allocs |
| --- | ---: | ---: | ---: |
| Scalars | 48 / 1 | 0 / 0 | 0 / 0 |
| Mixed | 51 / 2 | 3 / 1 | 3 / 1 |
| Nested | 48 / 1 | 0 / 0 | 0 / 0 |
| Binary1KiB | 51 / 2 | 3 / 1 | 3 / 1 |
| Binary64KiB | 51 / 2 | 3 / 1 | 3 / 1 |
| String1KiB | 2,096 / 3 | 1,024 / 1 | 1,024 / 1 |

Clearing destinations also preserves allocation parity: Mixed uses 6 B / 2 allocations in both libraries, String1KiB uses 1,026 B / 2, and Binary64KiB uses 65,539 B / 2. The cleared 64 KiB binary decoder takes 4,083 ns for ours and 3,567 ns for tinylib, a 1.14× gap; that case changed little from the baseline because the large allocation and copy dominate.

Scalars is a four-field, 35-byte map. Mixed is a six-field, 60-byte map containing uint64, string, bool, float64, binary, and a two-element int64 array. Nested has two child structs and occupies 26 bytes. The payload variants change only Mixed’s binary field to 1 KiB or 64 KiB, or its string to 1 KiB, giving 1,083, 65,597, and 1,083 wire bytes respectively. Go field names and map order match exactly across implementations.

## Fresh 1 KiB string encoding

Fresh means a new codec on every operation with warmed type metadata. Construction and the selected I/O buffers are included; caller input/output setup is excluded. Tinylib includes its Flush call.

| Implementation | Before ns/op | After ns/op | Before B/op / allocs | After B/op / allocs |
| --- | ---: | ---: | ---: | ---: |
| Ours, native | 1,634 | 227.7 | 32,864 / 3 | 1,120 / 3 |
| Ours, 4 KiB bufio | 1,901 | 516.8 | 37,024 / 5 | 5,280 / 5 |
| Tinylib, 4 KiB internal buffer | — | 327 | — | 4,144 / 2 |

The native improvement is 1,634 → 227.7 ns and 32,864 → 1,120 B. With matching 4 KiB buffering, ours improves from 1,901 → 516.8 ns and 37,024 → 5,280 B. Tinylib still has lower CPU time in that buffered comparison: 327 ns. The allocation count is unchanged for fresh encoding; the scratch allocation is much smaller.

## Warmed encoding and copy controls

All reused encode cases still report zero B/op and zero allocs/op. These main-suite encodes write to a counting sink that does not copy the output.

| Fixture | Before ns/op | After ns/op | Tinylib ns/op |
| --- | ---: | ---: | ---: |
| Scalars | 71.08 | 71.96 | 20.18 |
| Mixed | 129.4 | 131.4 | 42.11 |
| Nested | 66.28 | 67.9 | 16.97 |
| Binary1KiB | 129.5 | 131.7 | 54.89 |
| Binary64KiB | 133 | 135.3 | 41.5 |
| String1KiB | 142.2 | 143.3 | 53.72 |

The small native encode medians increased by roughly 1–2.5%; these changes do not close the 3–4× small-struct encoding gap. The timing ranges and all samples are preserved without discarding outliers.

A separate control copies the full encoded message into a pre-grown bytes.Buffer. It excludes the caller-owned output allocation equally for both libraries:

| Fixture | Before native ns/op | After native ns/op | After buffered ns/op | Tinylib ns/op |
| --- | ---: | ---: | ---: | ---: |
| Mixed | 164.8 | 167.3 | 171.8 | 44.03 |
| Binary64KiB | 975.2 | 924.1 | 1,037 | 837.2 |

For a real 64 KiB output copy, our updated native path takes about 1.10× tinylib’s time. These copy-heavy figures have more variation and do not establish a separate improvement from this change.

## Matching decoder buffer sizes

Both columns below use 4 KiB input buffering. Adding bufio to ours reduces underlying reads, but the per-field parsing and reflection costs remain.

| Fixture | Our buffered before | Our buffered after | Tinylib |
| --- | ---: | ---: | ---: |
| Scalars | 336.3 | 268.9 | 53.88 |
| Mixed | 586.8 | 473.1 | 98.12 |
| Nested | 327.4 | 265.5 | 52.49 |
| Binary1KiB | 603.9 | 490.4 | 115.3 |
| Binary64KiB | 1,387 | 1,335 | 959.8 |
| String1KiB | 731.5 | 548 | 176.8 |

The remaining native small-struct decode gap is about 4.5–4.6×. A separate updated Mixed decode CPU profile puts about 32% of total sampled CPU cumulatively in Reader.readFull, including underlying reads; errors.Is accounts for about 6% flat. Reflection dispatch, key parsing, and field access also remain. The shared child-plan cache lookups are no longer prominent in the top profile entries. Profiling followed all timing samples; these diagnostic shares are not predicted speedups.

## Validation and measurement

- Full repository `go test ./...`, `go test -race ./...`, and `go vet ./...` passed. HTTP integration tests used authorized localhost listener access. Formatting and diff checks passed.
- Allocation regressions require zero allocations for warmed scalar structs and reused binary fields, and at most one owned-string allocation for the measured warmed strings through 32 KiB. Tests also cover exact traversal-budget exhaustion in two consecutive roots, clean EOF afterward, ignored fields before exported fields, fragmented strings, shared Reader/Writer copies, owned-result independence, and scratch growth around 32 KiB. Existing truncation, limits, hooks, recursive-plan concurrency, and stored-failure checks pass.
- Three focused fuzz targets passed with two workers and a 15-second target each: FuzzDecoderReflection (427,313 executions), FuzzEncoderReflection (385,397), and FuzzStreamPayloadTransfer (362,829). Their exact logs are retained with the raw results.
- The comparison-module tests verify byte-for-byte wire equality, decoded values, input independence, and underlying read calls. All 66 updated benchmark cases also passed a one-iteration smoke run, including actual output-copy and buffered wire checks.
- Five sequential unprofiled 300 ms samples per case. The run contains 42 baseline cases, 42 updated cases, and 24 tinylib cases, or 108 cases / 540 samples. Library/version groups rotate and reverse between rounds. These are local medians, with no statistical significance claim. No tests or profiles ran during the measured samples.
- The baseline and updated executables use identical harness code, prepared pointers, input fixtures, buffer sizes, and output sinks. Both warm schema metadata before timing. Reused cases exclude construction and storage warm-up; clear-target variants clear fields inside timing. Fresh cases do not force codec pointers into an extra global sink, so compiler escape decisions contribute to their results.
- These valid-message CPU/allocation microbenchmarks do not measure cold plan construction, arbitrary maps/slices, malformed-input throughput, retained heap size, or file/network throughput. They compare our reflection API with generated tinylib code. Type metadata remains process-wide; traversal limits and payload limits still apply.

## Raw results and reproduction

The [raw audit output](decoder-string-optimization-results.txt) contains commands, all 540 samples, complete medians/ranges, source/binary hashes, fuzz logs, validation results, and the updated CPU profile summary.

The isolated comparison module, generated codecs, baseline/updated executables, CSVs, source patch, and rotating runner are retained for this session in:

```text
/private/tmp/messagepack-decoder-string-20261105.zygxeiye
```

Its go.mod points to this checkout and pins tinylib/msgp v1.6.5 without changing the repository module. From the comparison directory, rebuild the updated binary and repeat the rotating comparison against the preserved baseline:

```sh
export GOCACHE=/private/tmp/messagepack-go-build
export GOMODCACHE=/private/tmp/messagepack-library-comparison.jVX5vg/modcache
export GOPROXY=off
go test -c -o after.test .
python3 run.py
```

The baseline executable was built before editing production files. Recreating it requires building the same harness with the repository source at the recorded baseline commit. The current native struct benchmark can also be run independently:

```sh
GOCACHE=/private/tmp/messagepack-go-build go test -run '^$' \
  -bench '^BenchmarkReflectionStruct$' -benchmem -benchtime=300ms -count=5
```
