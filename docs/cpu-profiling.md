# CPU profiling after the allocation optimizations

The reader experiments below have since been applied. See the
[reader fast-path follow-up](reader-cpu-optimizations.md) for validation and
fresh before/after measurements. The tables here preserve the original baseline.

Measured on November 5, 2026, at production commit
`4432a357d5c0bd0adb4cdb749554b4723fd98ee6`, with Go 1.27.2 on darwin/arm64,
an Apple M3, and Go's default eight logical CPUs. This initial investigation added
benchmarks and documentation, leaving production code and module dependencies unchanged.

**Reflection is a substantial encoding cost, but it does not explain the whole
tinylib gap. Decoding spends most of its time outside generic traversal.**
Replacing generic traversal with a schema-specific control saves about 55% of
mixed-message encoding time and 29% of decoding time. The control still takes
about 3.2 times tinylib's decoding time. Ordinary caller buffering does not close
that gap in these small in-memory workloads.

## Separate traversal from the typed codec

The new [CPU benchmarks](../cpu_benchmark_test.go) compare reflection with
[explicit traversal](../cpu_benchmark_message_test.go). Both paths use the same
Reader/Writer implementation, cached encoded field-name fragments, field-name
lookup map, wire bytes, destination reuse, and owned string results. Encoding
performs the same 16 underlying writes for the small mixed record. The fixture
is a **60-byte** map using Go field names, distinct from the older 57-byte
application fixture with custom names.

The typed control removes reflected field access, recursive plan dispatch,
root type lookup, hook checks, traversal limits, and the generic decoder's tag
peek/replay. It therefore measures **reflection plus generic traversal**, rather
than the isolated cost of the `reflect` package. It handles valid messages for
this particular schema; it is not a proposed replacement public decoder.

Five unprofiled 200 ms samples per case give these medians:

| Fixture and operation | Reflection ns/op | Typed control ns/op | Time removed | B/op / allocs/op, both paths |
| --- | ---: | ---: | ---: | ---: |
| Mixed encode | 128.0 | 58.21 | 54.5% | 0 / 0 |
| Mixed decode | 438.0 | 310.9 | 29.0% | 3 / 1 |
| Binary64KiB encode, counting output | 131.2 | 61.44 | 53.2% | 0 / 0 |
| Binary64KiB decode | 1211 | 1080 | 10.8% | 3 / 1 |
| String1KiB encode | 138.8 | 67.75 | 51.2% | 0 / 0 |
| String1KiB decode | 495.2 | 372.1 | 24.9% | 1024 / 1 |

The roughly 70 ns encode and 125–130 ns decode traversal differences stay fairly
constant as payloads grow. Payload copying increasingly dominates large records.
Allocation counts remain identical, so fewer allocations do not explain the
control's speedup. The owned name still allocates in both decoders.

## Compare the actual streaming APIs

An isolated comparison module pins generated tinylib/msgp v1.6.5 codecs. Tests
verify identical wire output, decoded values, input independence, and I/O call
counts. Both implementations reuse warmed codecs and destinations. Tinylib
encoding includes Flush after every record; its internal buffer is 4 KiB.
Our native path uses its existing immediate, unbuffered streaming API.

| Fixture | Our encode ns/op | Tinylib encode ns/op | Our decode ns/op | Tinylib decode ns/op |
| --- | ---: | ---: | ---: | ---: |
| Scalars, four fields | 71.42 | 20.04 | 246.9 | 53.21 |
| Mixed, six fields | 130.0 | 41.96 | 437.3 | 97.09 |
| Nested structs | 66.10 | 17.03 | 241.2 | 52.27 |
| Binary64KiB | 133.4 | 41.78 | 1217 | 903.7 |
| String1KiB | 140.6 | 53.63 | 488.6 | 171.3 |

All these encodes allocate zero. Both decoders allocate zero for Scalars and
Nested, 3 B / one allocation for Mixed and Binary64KiB, and 1024 B / one
allocation for String1KiB. Allocation parity is confirmed for these warmed
workloads. General maps/slices, float32 conversion, hooks, cold construction, and
other input shapes are outside this comparison; it does not establish that
every allocation in every supported path is unavoidable.

The counting output does not copy payloads. A separate pre-grown bytes.Buffer
control copies the actual encoded output and includes Reset per message:

| Fixture | Our native ns/op | Our bufio ns/op | Tinylib ns/op |
| --- | ---: | ---: | ---: |
| Mixed | 165.0 | 171.3 | 43.83 |
| Binary64KiB | 979.4 | 987.6 | 821.9 |

The 64 KiB encode gap falls to about 1.19 times tinylib when the output is
actually copied. Counting-sink throughput for large payloads must not be read
as payload processing bandwidth.

## What the CPU profiles show

Six separate CPU profiles cover Mixed encoding and decoding for our reflection
path, tinylib, and our typed traversal control. Each uses an eight-second
benchmark target. Together they contain 57.15 CPU-seconds of sampled execution.
Profiled timings are excluded from the timing tables.

Representative shares of all samples in the corresponding reflection profile:

| Path | Function | Flat CPU | Cumulative CPU |
| --- | --- | ---: | ---: |
| Encode | reflectedEncoder.encodePlanned | 14.91% | 81.05% |
| Encode | reflectedEncoder.encodeStruct | 6.97% | 74.82% |
| Encode | Writer.write | 12.59% | 20.54% |
| Encode | lookupReflectionPlan | 0.37% | 8.92% |
| Encode | reflect.Value.Field | 3.55% | 5.50% |
| Decode | Reader.readFull | 18.94% | 33.41% |
| Decode | Reader.peekTag | 1.21% | 11.82% |
| Decode | runtime.mapaccess2_faststr | 1.69% | 8.56% |
| Decode | errors.Is | 4.83% | 4.83% |
| Decode | reflect.Value.Field | 1.45% | 1.93% |
| Decode | reflectedDecoder.decodePlanned | 4.10% | 79.13% |

**Cumulative samples include callees.** In particular, 81% under encodePlanned
or 79% under decodePlanned is not time spent doing reflection. Their descendants
include the actual codec and endpoint work. These overlapping percentages cannot
be summed or treated as predicted speedups. See Go's
[explanation of flat and cumulative profiles](https://go.dev/blog/pprof).

Encoding's costs include plan/hook dispatch, reflected field and array access,
cached-prefix emission, and repeated Writer calls. Root lookup still reaches
sync.Map once per record, accounting for about 9% cumulatively here. Statically
known children already have cached plans; adding another cache to every field
would not address the current code. Source profiles show checks around
sync.Once and indirect calls, rather than repeated struct metadata construction.

Decoding's read loop is a larger cost than reflected field access. It handles
partial reads, EOF precedence, no-progress protection, and a retained tag.
The generic decoder peeks each value's tag and the typed operation consumes it
through the same machinery. After removing generic traversal, readFull still
accounts for 42.41% cumulatively in the typed decode profile. Field-name parsing,
map lookup, and result ownership remain as well.

There is also runtime activity: for example, madvise is about 10% of samples in
both libraries' Mixed decode profiles. These profiles include allocator, GC,
scheduler, and harness work; they are not instruction-only codec measurements.
CPU sampling does not distinguish arithmetic from memory stalls.

## Why bufio alone does not reproduce tinylib

The small Mixed record uses 25 underlying reads and 16 writes with our native
codec. Both tinylib and our caller-buffered variant reach the source/sink once
per record. Nevertheless, our buffered Mixed encode takes 167.1 ns and decode
takes 467.8 ns, compared with native 130.0 ns and 437.3 ns.

Tinylib's generated decoder dispatches on map-key bytes from its internal buffer,
and its scalar readers consume that buffer directly. Its writer appends scalars
and cached keys into its own buffer before flushing. Our typed operations still
run their read/write checks and copy small fragments through the io interfaces
when a caller supplies bufio. Reducing the number of underlying calls therefore
does not remove the per-value machinery.

An internal buffer that the parser reads directly would be a different design
change. Read-ahead, flushing, and the current immediate-write behavior are public
contract decisions. These measurements do not establish that adding such a
buffer is the best next step for this repository.

## Small isolated reader experiments

Two changes were initially tested through Go source overlays, leaving production
files untouched at that stage. Their combined diff is retained as the
[experiment patch](cpu-reader-prototypes.patch), now applied in the
[reader fast-path follow-up](reader-cpu-optimizations.md):

1. Call errors.Is for EOF only when the underlying error is non-nil.
2. Return immediately from readScalarBody when the tag contains the entire value,
   avoiding a readFull call with an empty destination.

Each experiment uses the same benchmark executable source and a separately
measured control in five rotating/reversing 200 ms rounds:

| Decode fixture | Control ns/op | Nil-error guard | Empty-body return | Both |
| --- | ---: | ---: | ---: | ---: |
| Mixed | 446.3 | 413.1 | 423.5 | 402.3 |
| Binary64KiB | 1189 | 1169 | 1183 | 1166 |
| String1KiB | 502.2 | 472.8 | 487.6 | 474.4 |

The combined Mixed reduction is 9.9%; the individual reductions are 7.4% and
5.1%. Allocation counts and sizes are unchanged. Reader and Decoder tests pass
for all three prototype builds. Binary64KiB ranges overlap, and the String1KiB
combination is slightly slower than the nil-error guard alone. These are local
measurements without a significance claim; improvements are not simply additive.

## Recommended direction

Keep the ergonomic reflection API and existing typed foundation. The small
successful-read fast paths above are now applied: they address measured CPU cost
with little additional machinery and preserve the current I/O model.

Then measure a small per-codec root-plan cache and reductions in repeated plan,
hook, and tag dispatch. The encoder's root lookup is visible; the decoder's
root lookup is much smaller. Preserve recursive-type initialization, hook
addressability, nil behavior, traversal limits, and EOF handling in any such
change. These additional candidates have not been implemented or timed here.

Existing hooks provide a schema-specific option for applications with especially
hot records. Their by-value Reader/Writer scratch can still allocate, as the
[allocation report](allocation-profiling.md) documents, so hook performance needs
its own measurement. The typed benchmark bypasses hooks and does not claim to
measure their public call path.

For workloads dominated by large payloads, use the real-copy results when
prioritizing work: the fixed traversal overhead becomes a much smaller fraction
of total execution time.

## Audit and reproduction

The [raw audit output](cpu-profiling-results.txt) preserves all 300 unprofiled
samples, timing ranges, profile summaries, source listings, commands, runner
source, source/binary hashes, and validation results. The baseline comparison
has 48 cases with five samples each; reader experiments add 60 samples.
Independent timing groups rotate and reverse, and run sequentially. Tests,
builds, and CPU profiling run separately from those timing samples.

Validation passed: full repository go test ./... and go vet ./..., the native
benchmark smoke run, comparison wire/round-trip/ownership/I/O checks, and Reader
and Decoder tests in each overlay build. Full repository tests required
localhost listener access for the existing HTTP integration suite. No new
production behavior or API was introduced.

Reproduce the self-contained traversal control from the repository root:

```sh
export GOCACHE=/private/tmp/messagepack-go-build
go test -run '^$' -bench '^BenchmarkCPUTraversal$' -benchmem -benchtime=200ms -count=5
go test -c -o /private/tmp/messagepack-cpu.test .
/private/tmp/messagepack-cpu.test -test.run '^$' \
  -test.bench '^BenchmarkCPUTraversal$/^Mixed$/^Reflection$/^Decode$' \
  -test.benchtime=8s -test.cpuprofile=/private/tmp/messagepack-decode.cpu.pprof
go run cmd/pprof -top /private/tmp/messagepack-cpu.test /private/tmp/messagepack-decode.cpu.pprof
```

The toolchain in this environment omits the prebuilt go tool pprof executable;
building or running cmd/pprof from the installed Go source works. The comparison
module, generated tinylib codecs, matching executables, binary profiles, overlays,
and complete runners remain available for this session at:

```text
/private/tmp/messagepack-cpu-20261105.ezkm_8cr
```

Its comparison/go.mod points to this checkout. Rebuilding uses
GOMODCACHE=/private/tmp/messagepack-library-comparison.jVX5vg/modcache and
GOPROXY=off; tinylib is not added to the repository module. These successful,
warmed microbenchmarks exclude cold type-plan construction and do not model
file/socket latency, malformed-input throughput, or cross-platform performance.
