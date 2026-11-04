# Allocation profiling

> Historical baseline: short-string encoding and short-payload Skip have since
> been optimized in production. See the [applied before/after measurements](allocation-improvements.md).

Measured on 2026-11-04 against production commit
`97dd666993514f4eaddbd0b900d61e04234fc91d`, with Go 1.27.2,
darwin/arm64, macOS 26.5.2, and an Apple M3. Benchmarks used the default eight
logical CPUs. This report adds measurement coverage; production code is unchanged.

The clearest small optimization is to reuse existing nine-byte scratch for short
string writes. An isolated prototype reduced fresh encoding of the 57-byte
example message from **32856 to 88 B/op**, removing one allocation. Owned string
decoding and by-value hook scratch are the other confirmed sources of avoidable
allocation. Reused typed primitives and transfers already allocate zero in the
measured successful workloads.

## Evidence and boundaries

The [raw results](allocation-profile-results.txt) retain all 455 samples from the
91 existing streaming cases and all 310 samples from 62 new allocation cases.
Each case has five samples with a 100 ms target. Separate control/prototype runs
add 280 samples. Tables show medians, with no significance claim.

Six allocation profiles record both `alloc_space` and `alloc_objects`, plus
`inuse_space` after GC, using `-test.memprofilerate=1`. Two separate three-second
CPU profiles cover fresh message encoding and reused message decoding. Compiler
escape analysis corroborates the allocation sites. Profile summaries and relevant
escape diagnostics are retained in the raw results. Binary profiles and matching
test executables are local artifacts under
`/private/tmp/messagepack-allocation-20261104`.

Unprofiled benchmark samples supply timing and per-operation allocation numbers.
Profiling every allocation substantially perturbs timing. Profiles include test
setup, calibration, regexp matching, and runtime work; their totals are not codec
bytes per operation. Go's tiny allocator also packs small pointer-free values into
shared heap blocks, so profiled object counts need not equal benchmark
`allocs/op`. Retained space and cumulative allocation are different measurements.
Skip's reused payload case includes its first scratch allocation in the timed
batch; the reported zero B/op and allocs/op round down that one-time cost.

Existing benchmarks use generated input and counting sinks, with fixture and
caller-owned buffering setup outside timing. They do not measure file/socket
throughput or destination copying. `New` deliberately retains codec pointers in
global sinks; `Reset` includes reuse and prewarms encoding/transfer scratch.
See the [streaming workload descriptions](streaming-benchmarks.md) for details.
Historical timing results there predate this Reader/Writer refactor.

The new [benchmarks](../allocation_benchmark_test.go) cover:

- String writes at 0, 1, 8, 31, 32, 1024, 32768, and 32769 bytes, with fresh/reused
  codecs and unbuffered/caller-buffered output.
- Owned binary/string reads at 0, 1, 8, 31, 32, and 256 bytes, complementing the
  existing 1 KiB, 64 KiB, and 8 MiB cases. Results escape to typed global sinks.
- Repeated Encoder/Decoder hooks, including a synthetic no-I/O dispatch control,
  a scalar hook, and the same six-field message as the typed benchmarks. Hook
  targets are pointers prepared outside timing, avoiding per-call value boxing.
- Skip over a scalar, the 57-byte message, and 64 nested single-element arrays.
  The deepest fixture contains 65 wire bytes and stays within default limits.

## Current allocation costs

The following cases use unbuffered endpoints. The owned-result cases reuse a
Reader; hook cases reuse an Encoder/Decoder on consecutive successful calls.

| Case | Median ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| Primitive encode, New | 34.2 | 88 | 2 |
| Primitive decode, New | 97.6 | 120 | 2 |
| Message encode, New | 1606 | 32856 | 3 |
| Message encode, Reset | 105.7 | 0 | 0 |
| Message decode, New | 375.9 | 192 | 17 |
| Message decode, Reset | 352.6 | 72 | 15 |
| 64 KiB transfer encode, New | 17736 | 32856 | 3 |
| 64 KiB transfer encode, Reset | 16358 | 0 | 0 |
| 64 KiB transfer decode, New | 17634 | 32888 | 3 |
| 64 KiB transfer decode, Reset | 16353 | 0 | 0 |
| Owned 8-byte binary | 45.9 | 8 | 1 |
| Owned 8-byte string | 43.11 | 16 | 2 |
| Owned 64 KiB binary | 18768 | 65536 | 1 |
| Owned 64 KiB string | 22083 | 131072 | 2 |
| Scalar encode hook | 12.14 | 24 | 1 |
| Scalar decode hook | 21.15 | 24 | 1 |
| Message encode hook | 116.5 | 24 | 1 |
| Message decode hook | 365.2 | 96 | 16 |
| Skip message, New | 1790 | 32888 | 3 |
| Skip message, Reset | 269.4 | 0 | 0 |
| Skip depth 64, Reset | 609.2 | 0 | 0 |

### Short payload scratch dominates fresh codecs

`Writer.EncodeString` calls `payloadBuffer` for any nonempty string, allocating
32768 bytes even for a one-byte value. All seven strings in the example message
fit existing nine-byte scalar scratch. The fresh-message allocation profile
attributes **99.64% of all profiled bytes** to `Writer.payloadBuffer`.

`Reader.Skip` has the same issue when discarding a short string or binary value.
The fresh-message Skip profile attributes **99.55%** to `Reader.payloadBuffer`.
Scalar and deeply nested header-only skips need no payload buffer. Their reused
cases allocate zero, and escape analysis keeps the bounded traversal state off
the heap. The 256-frame array is not a measured heap-allocation problem.

### Owned strings copy twice

`readOwnedPayload` allocates a full payload slice, then `DecodeString` creates the
owned string with `string(payload)`. The 64 KiB profile attributes 64064 KiB to
each site, including calibration: the two payload-sized allocations have equal
cost. Only the final string remains reachable in the result sink after GC.

At 1 KiB and 64 KiB, strings allocate exactly twice the binary result size.
At 8 MiB, string reads allocate approximately 16 MiB per result. Binary reads
allocate approximately 8 MiB once, which supplies their required owned storage.
Empty reads allocate zero; one-byte strings avoid a separate string allocation
on this toolchain. Neither small-value exception generalizes to arbitrary input.

A follow-up could read short strings into existing scratch before making the
owned string. For larger strings, evaluate a pre-sized `strings.Builder` with
bounded reusable read scratch. That could remove the full temporary allocation,
but introduces retained scratch and still copies bytes. These are unmeasured
options; ownership, limits, bounded I/O, and failure precedence must remain intact.

### Hook copies escape through their inline scratch

The interfaces pass Reader/Writer by value. A hook using typed I/O passes that
copy's inline scratch to an `io.Reader`/`io.Writer` method. Escape analysis moves
the 24-byte copy onto the heap. Allocation profiles point directly at the hook
implementations. A no-I/O hook measures zero allocation, isolating the scratch
escape from interface dispatch itself.

For repeated messages, the hooks add 24 B and one allocation over typed calls:
encoding is 24 B/1 allocation versus zero; decoding is 96 B/16 allocations
versus 72 B/15 allocations. Changing schema field ownership or introducing a
borrowed public API is unnecessary to explain this difference.

## Isolated experiments

These patches record the isolated experiments from this profiling pass. The
short-string branch has since been applied; shared scratch remains an unapplied
experiment.
Each was built separately from the same baseline, passed the full existing test
suite, and was measured against a separate five-sample control run. Their effects
have not been combined.

The [short-string prototype](allocation-string-prototype.patch) adds a five-line
branch after writing the header. For nonempty strings of at most nine bytes, it
copies into existing scalar scratch and performs the same payload write. Larger
strings retain the existing path. It adds no state, pooling, unsafe conversion,
interface extension, or public API.

| Case | Control ns/op | Prototype ns/op | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: |
| Message encode, New | 1638 | 115.3 | 32856 → 88 | 3 → 2 |
| Message encode, Reset | 111 | 101.7 | 0 → 0 | 0 → 0 |
| 1-byte string, New | 1527 | 23.05 | 32856 → 88 | 3 → 2 |
| 8-byte string, New | 1585 | 22.92 | 32856 → 88 | 3 → 2 |

The 32768-byte reduction is **99.73%** of fresh-message allocation bytes. This
helps short-lived codecs most. Reused codecs already allocate zero. Timing is
mixed at the nanosecond scale: the one-byte reused case rose from 11.01 to
15.58 ns/op, while the eight-byte case fell from 10.01 to 9.337 ns/op. These short
samples do not establish universal throughput improvement.

The [shared-scratch experiment](allocation-scratch-prototype.patch) moves each
nine-byte array into existing shared state and adds Writer readiness guards before
accessing it. It also adjusts a fuzz test's private scratch access. This avoids
moving each hook's Reader/Writer copy to the heap without changing hook signatures.

| Case | Control ns/op | Prototype ns/op | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: |
| Scalar encode hook | 12.18 | 7.599 | 24 → 0 | 1 → 0 |
| Scalar decode hook | 21.21 | 16.2 | 24 → 0 | 1 → 0 |
| Message encode hook | 116.4 | 113.6 | 24 → 0 | 1 → 0 |
| Message decode hook | 366.6 | 384.3 | 96 → 72 | 16 → 15 |

The allocation savings are consistent, but the timing tradeoff is mixed. Reused
primitive encoding rose from 20.84 to 23.36 ns/op, and typed message decoding rose
from 357.6 to 379.0 ns/op. The added checks and shared-state access deserve review.
This broader refactor is a lower priority than the small string branch.

## Recommended order

1. Apply the short-string write change with focused boundary, ownership, error,
   and allocation checks. It removes a 32 KiB allocation with very little code.
2. Apply the same existing-scratch idea to short skipped payloads and short owned
   strings. Measure each independently; no savings for these variants are claimed
   by the current prototypes.
3. Evaluate large owned string decoding if applications materialize large strings.
   Existing transfer APIs already avoid payload-sized result storage when callers
   can stream to a destination.
4. Revisit shared scratch only if hook traffic makes its allocation meaningful.
   Keep the measured timing tradeoffs visible.

Pooling, larger inline buffers, new borrowed APIs, and changing the 32 KiB transfer
buffer are unnecessary for the first improvement. Fresh 1 KiB transfers still pay
for a 32 KiB buffer, but reused 1 KiB/64 KiB/8 MiB transfers allocate zero. The
five repeated allocation-byte and retained-memory checks passed: retained scratch
remained 32768 bytes per warmed codec, independent of total payload size.

CPU profiles support investigating string decoding: `runtime.mallocgc` accounted
for 22.74% cumulative samples in reused message decoding. Fresh encoding's CPU
profile was dominated by Darwin runtime/synchronization frames and was noticeably
perturbed; it is not a reliable breakdown of codec instruction costs. Allocation
profiles provide the clearer evidence for the first change.

## Reproduce

Run timing benchmarks without concurrent tests or profiling. No additional Go
dependencies or profiling packages are required.

```sh
export GOCACHE=/private/tmp/messagepack-go-cache
profile_dir=$(mktemp -d /private/tmp/messagepack-profile.XXXXXX)
go test -c -o "$profile_dir/messagepack.test" .
"$profile_dir/messagepack.test" -test.run '^$' -test.bench '^Benchmark(Stream|Allocation)' -test.benchmem -test.benchtime=100ms -test.count=5

"$profile_dir/messagepack.test" -test.run '^$' -test.bench '^BenchmarkStreamMessage/Encode/Unbuffered/New$' -test.benchtime=10000x -test.memprofilerate=1 -test.memprofile "$profile_dir/message.mem.pprof"
go tool pprof -top -alloc_space "$profile_dir/messagepack.test" "$profile_dir/message.mem.pprof"
go tool pprof -top -alloc_objects "$profile_dir/messagepack.test" "$profile_dir/message.mem.pprof"
go tool pprof -top -inuse_space "$profile_dir/messagepack.test" "$profile_dir/message.mem.pprof"

"$profile_dir/messagepack.test" -test.run '^$' -test.bench '^BenchmarkStreamMessage/Decode/Unbuffered/Reset$' -test.benchtime=3s -test.cpuprofile "$profile_dir/decode.cpu.pprof"
go tool pprof -top "$profile_dir/messagepack.test" "$profile_dir/decode.cpu.pprof"
go test -run '^$' -gcflags=-m=2 . > "$profile_dir/escape.txt" 2>&1
```

The raw results preserve the exact commands for the other profiles and prototype
comparisons. To reproduce a prototype, start from the recorded production commit
in a temporary directory, copy the allocation benchmark and fixture files into it,
and apply only its patch with `git apply` there. Build a separate test binary;
keep its profiles paired with that binary. The shared-scratch patch also includes
its one test-fixture adjustment. Compare against the same baseline without a patch.

Validation passed: `go test ./...`, `go test -race ./...`, `go vet ./...`, and five
repetitions of existing allocation, retained-memory, scratch-bound, and benchmark
fixture tests. Full and race tests used authorized localhost-listener access for
the HTTP integration tests. Measurements cover this darwin/arm64 environment and
synthetic endpoints; they do not establish cross-platform performance or a blanket
zero-allocation guarantee.
