# Allocation profiling follow-up

Measured on 2026-11-04 against production commit
`6804501655d9475ce0a6cd5fecd3c9cf04171295`, using Go 1.27.2 on darwin/arm64
with an Apple M3 and the default eight logical CPUs.

This pass applies one small change: owned string reads of at most nine bytes use
the Reader's existing scalar scratch before copying into the returned string.
The payload reader was renamed from `readOwnedPayload` to `readPayload` to reflect
that its internal string staging may now borrow scratch. Binary results retain
their owned allocation. No public API or state layout changes are needed.

The existing read loop, header validation, limits, I/O request sizes, and terminal
failure handling are unchanged. The returned string still owns its contents.

## Before and after

These are medians of five unprofiled samples per version, each with a 200 ms
target. The message is the existing 57-byte schema with seven short strings.
Negative timing changes mean less time per operation.

| Case | Before ns/op | After ns/op | Time change | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Owned 1-byte string | 32.14 | 28.49 | -11.4% | 1 → 0 | 1 → 0 |
| Owned 2-byte string | 41.40 | 36.04 | -12.9% | 4 → 2 | 2 → 1 |
| Owned 8-byte string | 42.18 | 37.96 | -10.0% | 16 → 8 | 2 → 1 |
| Owned 9-byte string | 44.46 | 39.47 | -11.2% | 32 → 16 | 2 → 1 |
| Owned 10-byte string | 44.43 | 46.15 | +3.9% | 32 → 32 | 2 → 2 |
| Owned 8-byte binary | 44.87 | 46.74 | +4.2% | 8 → 8 | 1 → 1 |
| Message decode, New | 379.4 | 344.4 | -9.2% | 192 → 160 | 17 → 10 |
| Message decode, Reset | 354.7 | 320.0 | -9.8% | 72 → 40 | 15 → 8 |
| Message decode, buffered New | 409.9 | 367.9 | -10.2% | 192 → 160 | 17 → 10 |
| Message decode, buffered Reset | 385.0 | 344.7 | -10.5% | 72 → 40 | 15 → 8 |
| Message decode hook | 362.8 | 323.4 | -10.9% | 96 → 64 | 16 → 9 |
| Scalar decode hook | 21.08 | 20.89 | -0.9% | 24 → 24 | 1 → 1 |
| Primitive decode, Reset | 79.20 | 80.42 | +1.5% | 0 → 0 | 0 → 0 |
| Message encode, Reset | 100.7 | 101.0 | +0.3% | 0 → 0 | 0 → 0 |
| Owned 64 KiB string | 21733 | 21756 | +0.1% | 131073 → 131073 | 2 → 2 |

Reused message decoding removes seven allocations and 32 bytes per message:
46.7% fewer allocations and 44.4% fewer allocated bytes. One-byte strings use Go's
existing single-byte string storage on this toolchain; arbitrary longer owned
strings still need their final allocation.

There is a small timing tradeoff. The binary cases from zero through 256 bytes
measured 0.9–2.3 ns slower, with unchanged allocation counts and sizes. The added
selection branch runs on both payload types. Ten-byte strings also measured
slower, while the 64 KiB control changed by 0.1%. These are local synthetic
measurements with no confidence interval or cross-platform throughput claim.

## Profiling and remaining opportunities

Four allocation workloads were profiled before and after, with every allocation
sampled: reused message decoding, an owned eight-byte string, an owned 64 KiB
string, and a scalar decode hook. Both `alloc_space` and `alloc_objects` were
inspected. Short string allocation moves to the final `DecodeString` copy; the
temporary payload allocation is removed. Compiler escape diagnostics confirm the
remaining owned string allocation and the escaping codec copies in scalar hooks.

Larger owned strings still allocate a payload slice and the final string. The
64 KiB profiles attribute approximately half of the payload allocation bytes to
each. Reducing that cost needs a different string construction path; it remains
future work if large owned strings are a significant workload.

Hooks that perform typed I/O still add 24 B and one allocation for their by-value
Reader/Writer copy. No-I/O hooks allocate zero. The earlier
[shared-scratch experiment](allocation-profiling.md#isolated-experiments) required
more implementation changes and had mixed timing results; it remains deferred.
Reused primitive encoding and decoding already allocate zero in these workloads.

## Method and validation

Both test executables use identical benchmark code, including new two-, nine-,
and ten-byte owned-read cases. There are 32 cases and 320 samples total. Each
sample runs all cases for one version; version order alternates before/after,
after/before, before/after, after/before, before/after. Timed comparisons ran
sequentially, without concurrent tests or profiling. The
[raw samples](allocation-followup-results.txt) include benchmark patterns, order,
iteration counts, timing, and allocation measurements.

Owned-read cases reuse a Reader and publish their results to global sinks. New
message cases include escaping codec construction; Reset cases reuse a codec.
Caller buffering uses 4 KiB. Sources generate bytes or replay literal fixtures;
destinations count output. These do not model file or socket throughput.

Allocation profiles use 10000 iterations, except the 64 KiB case uses 1000, with
`-test.memprofilerate=1`. Profiled timings are excluded from the comparison.
Profiles include harness allocations, and Go's tiny allocator can combine small
objects into one profiled block; profile totals are not per-operation costs.
Matching binaries, full benchmark output, profiles, profile summaries, and escape
diagnostics are retained in `/private/tmp/messagepack-profile-round2` for this
session.

Validation passed:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `go test -run '^$' -fuzz '^FuzzStreamTypedDecode$' -fuzztime=10s -parallel=2 .`
  (215688 executions, including the initial corpus).
- Formatting and diff checks.

Tests add fragmented reads and truncation at the scratch boundary, ownership
after source mutation and scratch reuse, and a maximum of one allocation for
short owned strings. Existing empty binary, raw byte, wider header, limit, source
error, EOF, and integration coverage also passes. HTTP tests used authorized
localhost listener access.

## Reproduce

Build a before executable with `reader.go` from the recorded baseline commit,
keeping the current benchmark files. Build an after executable with the current
implementation. For each pattern listed in the raw samples, run both binaries
five times in the alternating order above:

```sh
export GOCACHE=/private/tmp/messagepack-go-build
go test -c -o /private/tmp/messagepack-after.test .
/private/tmp/messagepack-after.test -test.run '^$' -test.bench '^BenchmarkAllocationOwnedSmall$' -test.benchmem -test.benchtime=200ms -test.count=1
```

To inspect an allocation profile independently of timing:

```sh
/private/tmp/messagepack-after.test -test.run '^$' -test.bench '^BenchmarkAllocationOwnedSmall$/^Bytes8$/^String$' -test.benchtime=10000x -test.memprofilerate=1 -test.memprofile /private/tmp/messagepack-short-string.pprof
go tool pprof -top -alloc_space /private/tmp/messagepack-after.test /private/tmp/messagepack-short-string.pprof
go tool pprof -top -alloc_objects /private/tmp/messagepack-after.test /private/tmp/messagepack-short-string.pprof
```
