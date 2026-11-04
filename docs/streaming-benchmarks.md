# Streaming memory and throughput measurements

> These recorded samples predate the 2026-11-04 Reader/Writer refactor and slice API removal.
> Current benchmark fixtures generate headers and integers through Writer methods;
> rerun the benchmarks to measure the current implementation and harness.

Measured on 2026-11-02 with `go1.27.2 darwin/arm64`, Darwin 25.5.0,
and an Apple M3. Go used its default eight logical CPUs (`-8` result suffix).
The module, toolchain, dependencies, and production code were unchanged.
These are local measurements of the workloads below, not timing requirements or
a comparison with another implementation. The [slice baseline](poc-benchmarks.md)
is a separate historical measurement with different ownership and I/O costs.

## Reproduce and audit

From the module root:

```sh
GOCACHE=/private/tmp/messagepack-go-cache go test -run '^$' -bench '^BenchmarkStream' -benchmem -benchtime=100ms -count=5 ./...
GOCACHE=/private/tmp/messagepack-go-cache go test -run '^Test(StreamTransferAllocationBytes|StreamRetainedWorkingMemory|StreamTransferScratchBounds|StreamBenchmark|ZeroAllocations|OwnedStringAllocations)' -v -count=5 ./...
```

The [raw audit output](streaming-benchmark-results.txt) contains the commands,
environment, all individual benchmark samples, and the verbose memory and slice
allocation checks. The benchmark command deliberately uses a **100 ms target per
sample**, rather than Go's default one second. Go calibrates the iteration count;
this is not a fixed operation count or an exact wall-clock limit. Every one of the
91 benchmark cases has five samples (455 samples total). Results below give the
median, minimum, and maximum timing; no confidence interval or significance test
is claimed. The cache override only makes the workspace cache writable.

## Workloads and timing boundaries

- `StreamPrimitives` processes six consecutive typed values: nil, true, -32769,
  maximum uint64, float32 1.5, and float64 1.5. Together they occupy 30 wire bytes.
- `StreamMessage` processes the existing literal 57-byte six-field schema with ID
  7, name `Ada`, active true, score 1.5, binary `{1, 2}`, and samples `[-1, 2]`.
  Schema dispatch and validation use the existing explicit test helpers. Decode
  owns its field names, name, and binary result; it has different ownership costs
  from the slice example's borrowed binary.
- `StreamContainer` traverses generated arrays of 1024 or 65536 consecutive
  unsigned integers beginning at zero. No array or encoded object is collected.
  Decode includes an equality check for every integer. Its source generates wire
  integers on demand into nine-byte scratch. The `Generator` cases separately
  measure that source feeding a counting sink in 32 KiB chunks, without decoding.
- `StreamTransfer` measures `EncodeBytesFrom`, `EncodeStringFrom`, `DecodeBytesTo`,
  and `DecodeStringTo` at **1024, 65536, and 8388608 payload bytes**. Sources fill
  requested chunks with `x`; decoder sources generate their family header too.
  Only the small header and fixed scratch are retained. Destinations count calls
  and byte lengths, without scanning, copying, or retaining payloads. Checks cover
  errors, transferred lengths, total output, and source boundaries. Existing
  correctness tests separately validate transferred contents byte by byte.
- `StreamOwned` decodes generated strings and binary at the same three sizes,
  using an existing Decoder and Reset. Returned data escapes through typed global
  sinks. These operations intentionally materialize results. The default 16 MiB
  owned-result limit admits the largest individual 8 MiB result.
- `StreamGenerator` separately measures the raw payload generator and counting
  sink with a preallocated 32 KiB copy buffer. Generation is timed in transfer and
  owned cases too. This baseline exposes harness cost; it is not subtracted from
  other results. Array generation, schema checks, function calls, counter updates,
  error checks, and Reset overhead remain in their respective measured loops.

All `MB/s` figures use **wire bytes per operation**, except the raw payload
generator, which uses payload bytes. Payload headers are three bytes at 1 KiB and
five at 64 KiB and 8 MiB. Container sizes are recorded in the results below.
Throughput describes in-process generated input and counting sinks; it does not
measure sockets, files, syscalls, destination copying, or durable writes.

`New` constructs a codec for every operation, including its first-use scratch
allocation. Codec pointers deliberately escape to global sinks so this scenario
includes heap-resident construction. It is not a claim that every caller's
constructor must escape. `Reset` uses one codec constructed outside timing and
includes Reset in every iteration. Encoders and transfer Decoders are prewarmed
to allocate reusable scratch before timing. Scalar and owned-result Decoders do
not need transfer scratch. Owned results still allocate on every owned read.

`Buffered4KiB` uses caller-owned `bufio` wrappers constructed outside timing.
Record encoding wraps its destination; record decoding wraps its source.
Transfers wrap **both** the source and destination. The source wrapper is Reset
for each generated value, and caller Flush is included after every encoded record
or transfer. Thus `New` compares codec construction with the same already-created
caller wrappers. It does not include wrapper construction or imply codec flushing.
This finite-value benchmark resets the wrapper at a known boundary; applications
processing consecutive values must keep using their original wrappers.

`readcalls/op` and `writecalls/op` count calls reaching the underlying generated
source or counting sink, after caller buffering. They are not codec method calls
or operating-system calls. The unused direction is reported as zero. Fixed input
and wrapper sizes make these counts deterministic in all five samples.

## Allocation bytes and retained working memory

The benchmark `B/op` observations and an independent `runtime.MemStats.TotalAlloc`
experiment both measure allocation **bytes**, not just allocation counts.
`TestStreamTransferAllocationBytes` takes before/after counters around 16 transfers
for each size, direction, family, and New/Reset scenario. Generation and sinks
require no payload-sized storage. It runs sequentially, with GC before each batch
and logging outside the measured interval. TotalAlloc is process-wide, so this
experiment allows runtime noise; exact benchmark byte counts are not asserted.

The regression budgets are at most 128 KiB allocated per transfer and at most
64 KiB spread across the three sizes. These generous bounds permit several fixed
scratch allocations and ordinary allocator variation while detecting allocation
proportional to the 8 MiB payload. They do not require zero allocations or freeze
constructor sizes. The repeated Reset test checks retained scratch capacities
stay at or below 32768 bytes for both families and both codec directions; it does
not require private buffer identity or a particular allocation strategy.

`TestStreamRetainedWorkingMemory` separately keeps **32 live Encoder/Decoder
pairs** after string or binary transfers at each size. It detaches their streams
with Reset(nil), runs GC, measures `HeapAlloc`, and uses `runtime.KeepAlive` on both
cohorts after the measurement. This measures reachable codec storage independently
of source length and output ownership. The observed scratch capacities and live
heap deltas are reported below. Heap deltas include unrelated runtime/test heap
changes and may undercount or overcount the cohort by that noise; they are not
exact object-size guarantees. The regression upper bound is 5 MiB per 32-pair
cohort: twice its 2 MiB bounded scratch plus 1 MiB noise allowance. No exact heap
delta or timing is asserted. Both allocation and retained-memory checks would
reject a whole 8 MiB payload retained per codec.

No heap profile was taken: byte counters, post-GC live cohorts, and directly
observed bounded capacities answer the scoped scaling question. These checks do
not prove all error paths, concurrent process activity, or arbitrary third-party
readers/writers have identical memory costs. No pooling, unsafe code, production
optimization, or toolchain change was introduced to obtain the results.

The existing `TestZeroAllocations` remains unchanged and verifies its four slice
API batches (primitive reads, borrowed payload reads, header reads, and sufficiently
preallocated appends). `TestOwnedStringAllocations` still reports its owned-slice
string exception. Stream measurements do not broaden those guarantees.

## Results

The benchmark command passed in **61.525 seconds**. All timing values include
the harness boundaries above. Allocation and call-count columns show ranges
when the five samples differ. Case names below omit the `BenchmarkStream`
prefix and `-8` platform suffix. The arrays occupy **2691** and **196229**
wire bytes respectively.

Fresh transfers measured **32848 B/op, 2 allocs/op** for Encoder and
**32880 B/op, 2 allocs/op** for Decoder at all three sizes, in both families
and buffering modes. These equal one 32768-byte scratch allocation plus the
measured escaping codec object. Warm Reset transfers measured **0 B/op and
0 allocs/op** for these successful inputs. This is a scoped observation, not
a blanket zero-allocation API promise.

Owned binary reads measured approximately one payload allocation; owned string
reads measured two payload-sized allocations (temporary bytes plus the safe
owned string conversion). At 8 MiB the observed ranges were 8388615–8388625
B/op and 16777228–16777236 B/op respectively. Small extra bytes are measurement
noise, not a change in the proportional ownership cost. The schema Decoder
measured 72 B/op and 15 allocs/op even with Reset because its short owned fields
are materialized on every message. A fresh schema Encoder uses 32848 B/op for
its lazily allocated string scratch and codec, even though the message is small.

At 8 MiB the generator baseline reached median 4017.83 MB/s; unbuffered Reset
transfers reached medians 4002.07–4017.44 MB/s. The similar times expose the
generation-dominated workload and do not establish isolated codec throughput.
Caller buffering reduces the 65536-element array from 65537 to 48 underlying
writes, and from 130946 to 48 underlying reads. It was slower in median time
with these very cheap counting endpoints; fewer calls do not by themselves
predict faster in-memory execution or real transport performance.

### Allocation-byte batches

Each cell gives the minimum–maximum B/transfer across five separate 16-transfer
batches. Zero cells are measured results, not test requirements.

| Operation/lifecycle | 1 KiB | 64 KiB | 8 MiB |
| --- | ---: | ---: | ---: |
| `EncodeBytes/New` | 32848 | 32848–32849 | 32848 |
| `EncodeBytes/Reset` | 0 | 0 | 0 |
| `EncodeString/New` | 32848 | 32848 | 32848 |
| `EncodeString/Reset` | 0 | 0 | 0 |
| `DecodeBytes/New` | 32880 | 32880 | 32880 |
| `DecodeBytes/Reset` | 0 | 0 | 0 |
| `DecodeString/New` | 32880 | 32880 | 32880 |
| `DecodeString/Reset` | 0 | 0 | 0 |

### Retained live cohorts

Each row summarizes five post-GC measurements, with 32 simultaneously live
Encoder/Decoder pairs. Scratch is the sum of all observed buffer capacities.
The small heap-delta differences across sizes are within measurement noise;
both the measured byte totals and the direct capacity bounds remain stable.

| Family/size | Retained scratch B | Median heap delta B | Heap delta min–max B |
| --- | ---: | ---: | ---: |
| `Bytes/1KiB` | 2097152 | 2062640 | 2062640 |
| `Bytes/64KiB` | 2097152 | 2063472 | 2063472 |
| `Bytes/8MiB` | 2097152 | 2063472 | 2063472 |
| `String/1KiB` | 2097152 | 2063280 | 2063280–2063392 |
| `String/64KiB` | 2097152 | 2063472 | 2063472–2063488 |
| `String/8MiB` | 2097152 | 2063472 | 2063472–2063584 |

### Primitive sequences, schema messages, and generated containers

| Case | Median ns/op | Min–max ns/op | Median MB/s | B/op | allocs/op | reads/op | writes/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `Primitives/Encode/Unbuffered/New` | 28.1 | 27.85–43.32 | 1067.66 | 80 | 1 | 0 | 6 |
| `Primitives/Encode/Unbuffered/Reset` | 19.88 | 19.8–20.14 | 1509.16 | 0 | 0 | 0 | 6 |
| `Primitives/Encode/Buffered4KiB/New` | 42.39 | 42.09–43.29 | 707.7 | 80 | 1 | 0 | 1 |
| `Primitives/Encode/Buffered4KiB/Reset` | 34.82 | 34.43–35.86 | 861.6 | 0 | 0 | 0 | 1 |
| `Primitives/Decode/Unbuffered/New` | 94.11 | 93.96–94.8 | 318.79 | 112 | 1 | 10 | 0 |
| `Primitives/Decode/Unbuffered/Reset` | 79.55 | 79.13–80.65 | 377.11 | 0 | 0 | 10 | 0 |
| `Primitives/Decode/Buffered4KiB/New` | 109.3 | 108.4–110.4 | 274.55 | 112 | 1 | 1 | 0 |
| `Primitives/Decode/Buffered4KiB/Reset` | 92.72 | 92.08–93.37 | 323.55 | 0 | 0 | 1 | 0 |
| `Message/Encode/Unbuffered/New` | 1652 | 1603–1754 | 34.51 | 32848 | 2 | 0 | 23 |
| `Message/Encode/Unbuffered/Reset` | 105.5 | 104.4–127.2 | 540.09 | 0 | 0 | 0 | 23 |
| `Message/Encode/Buffered4KiB/New` | 1672 | 1635–1685 | 34.09 | 32848 | 2 | 0 | 1 |
| `Message/Encode/Buffered4KiB/Reset` | 158.5 | 156–165.1 | 359.57 | 0 | 0 | 0 | 1 |
| `Message/Decode/Unbuffered/New` | 365.3 | 362.1–366.3 | 156.03 | 184 | 16 | 25 | 0 |
| `Message/Decode/Unbuffered/Reset` | 345.6 | 343.7–349.3 | 164.91 | 72 | 15 | 25 | 0 |
| `Message/Decode/Buffered4KiB/New` | 395.4 | 392.5–397.5 | 144.15 | 184 | 16 | 1 | 0 |
| `Message/Decode/Buffered4KiB/Reset` | 377.6 | 374.2–382.1 | 150.94 | 72 | 15 | 1 | 0 |
| `Container/Elements1024/Generator` | 3859 | 3761–3921 | 697.25 | 0 | 0 | 1 | 1 |
| `Container/Elements1024/Encode/Unbuffered/New` | 3722 | 3481–3960 | 722.98 | 80 | 1 | 0 | 1025 |
| `Container/Elements1024/Encode/Unbuffered/Reset` | 3683 | 3556–3731 | 730.67 | 0 | 0 | 0 | 1025 |
| `Container/Elements1024/Encode/Buffered4KiB/New` | 5378 | 5346–5495 | 500.33 | 80 | 1 | 0 | 1 |
| `Container/Elements1024/Encode/Buffered4KiB/Reset` | 5388 | 5359–5522 | 499.45 | 0 | 0 | 0 | 1 |
| `Container/Elements1024/Decode/Unbuffered/New` | 21830 | 21730–22140 | 123.27 | 112 | 1 | 1922 | 0 |
| `Container/Elements1024/Decode/Unbuffered/Reset` | 21892 | 21776–22864 | 122.92 | 0 | 0 | 1922 | 0 |
| `Container/Elements1024/Decode/Buffered4KiB/New` | 23488 | 23182–24701 | 114.57 | 112 | 1 | 1 | 0 |
| `Container/Elements1024/Decode/Buffered4KiB/Reset` | 23513 | 23278–23543 | 114.45 | 0 | 0 | 1 | 0 |
| `Container/Elements65536/Generator` | 246266 | 241269–254914 | 796.82 | 0 | 0 | 6 | 6 |
| `Container/Elements65536/Encode/Unbuffered/New` | 222074 | 220775–230099 | 883.62 | 80 | 1 | 0 | 65537 |
| `Container/Elements65536/Encode/Unbuffered/Reset` | 222862 | 222333–225106 | 880.49 | 0 | 0 | 0 | 65537 |
| `Container/Elements65536/Encode/Buffered4KiB/New` | 342011 | 340389–344561 | 573.75 | 80 | 1 | 0 | 48 |
| `Container/Elements65536/Encode/Buffered4KiB/Reset` | 345039 | 341586–358694 | 568.71 | 0 | 0 | 0 | 48 |
| `Container/Elements65536/Decode/Unbuffered/New` | 1457090 | 1447801–1840701 | 134.67 | 112 | 1 | 130946 | 0 |
| `Container/Elements65536/Decode/Unbuffered/Reset` | 1465462 | 1444602–1472953 | 133.9 | 0 | 0 | 130946 | 0 |
| `Container/Elements65536/Decode/Buffered4KiB/New` | 1552600 | 1541900–1605835 | 126.39 | 112 | 1 | 48 | 0 |
| `Container/Elements65536/Decode/Buffered4KiB/Reset` | 1530523 | 1518797–1546219 | 128.21 | 0 | 0 | 48 | 0 |

### Reader-to-writer transfers

| Case | Median ns/op | Min–max ns/op | Median MB/s | B/op | allocs/op | reads/op | writes/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `Transfer/1KiB/EncodeBytes/Unbuffered/New` | 1766 | 1758–1793 | 581.67 | 32848 | 2 | 1 | 2 |
| `Transfer/1KiB/EncodeBytes/Unbuffered/Reset` | 279.9 | 279.5–286.6 | 3669.8 | 0 | 0 | 1 | 2 |
| `Transfer/1KiB/EncodeBytes/Buffered4KiB/New` | 1849 | 1826–1880 | 555.45 | 32848 | 2 | 1 | 1 |
| `Transfer/1KiB/EncodeBytes/Buffered4KiB/Reset` | 312.4 | 311.6–313.6 | 3287.45 | 0 | 0 | 1 | 1 |
| `Transfer/1KiB/EncodeString/Unbuffered/New` | 1772 | 1732–1896 | 579.55 | 32848 | 2 | 1 | 2 |
| `Transfer/1KiB/EncodeString/Unbuffered/Reset` | 280.5 | 279.8–285.7 | 3661.54 | 0 | 0 | 1 | 2 |
| `Transfer/1KiB/EncodeString/Buffered4KiB/New` | 1874 | 1842–1883 | 547.89 | 32848 | 2 | 1 | 1 |
| `Transfer/1KiB/EncodeString/Buffered4KiB/Reset` | 313.5 | 313.1–349.5 | 3275.4 | 0 | 0 | 1 | 1 |
| `Transfer/1KiB/DecodeBytes/Unbuffered/New` | 1856 | 1843–1988 | 553.28 | 32880 | 2 | 3 | 1 |
| `Transfer/1KiB/DecodeBytes/Unbuffered/Reset` | 294.8 | 294.6–296 | 3483.27 | 0 | 0 | 3 | 1 |
| `Transfer/1KiB/DecodeBytes/Buffered4KiB/New` | 1889 | 1872–1912 | 543.8 | 32880 | 2 | 1 | 1 |
| `Transfer/1KiB/DecodeBytes/Buffered4KiB/Reset` | 324.5 | 324.3–336.8 | 3164.52 | 0 | 0 | 1 | 1 |
| `Transfer/1KiB/DecodeString/Unbuffered/New` | 1845 | 1842–1862 | 556.66 | 32880 | 2 | 3 | 1 |
| `Transfer/1KiB/DecodeString/Unbuffered/Reset` | 294.7 | 294.2–295.9 | 3485.1 | 0 | 0 | 3 | 1 |
| `Transfer/1KiB/DecodeString/Buffered4KiB/New` | 1904 | 1890–2073 | 539.38 | 32880 | 2 | 1 | 1 |
| `Transfer/1KiB/DecodeString/Buffered4KiB/Reset` | 325.1 | 324.4–325.2 | 3158.95 | 0 | 0 | 1 | 1 |
| `Transfer/64KiB/EncodeBytes/Unbuffered/New` | 17750 | 17685–17794 | 3692.48 | 32848 | 2 | 2 | 3 |
| `Transfer/64KiB/EncodeBytes/Unbuffered/Reset` | 16340 | 16305–16854 | 4011 | 0 | 0 | 2 | 3 |
| `Transfer/64KiB/EncodeBytes/Buffered4KiB/New` | 17799 | 17733–17837 | 3682.28 | 32848 | 2 | 2 | 3 |
| `Transfer/64KiB/EncodeBytes/Buffered4KiB/Reset` | 16419 | 16399–16427 | 3991.8 | 0 | 0 | 2 | 3 |
| `Transfer/64KiB/EncodeString/Unbuffered/New` | 17718 | 17695–17762 | 3699.17 | 32848 | 2 | 2 | 3 |
| `Transfer/64KiB/EncodeString/Unbuffered/Reset` | 16344 | 16320–16824 | 4010.03 | 0 | 0 | 2 | 3 |
| `Transfer/64KiB/EncodeString/Buffered4KiB/New` | 17794 | 17745–21126 | 3683.35 | 32848 | 2 | 2 | 3 |
| `Transfer/64KiB/EncodeString/Buffered4KiB/Reset` | 16477 | 16444–16584 | 3977.77 | 0 | 0 | 2 | 3 |
| `Transfer/64KiB/DecodeBytes/Unbuffered/New` | 17741 | 17664–18408 | 3694.38 | 32880 | 2 | 4 | 2 |
| `Transfer/64KiB/DecodeBytes/Unbuffered/Reset` | 16342 | 16325–16392 | 4010.48 | 0 | 0 | 4 | 2 |
| `Transfer/64KiB/DecodeBytes/Buffered4KiB/New` | 17770 | 17734–17834 | 3688.28 | 32880 | 2 | 3 | 2 |
| `Transfer/64KiB/DecodeBytes/Buffered4KiB/Reset` | 16458 | 16434–16517 | 3982.23 | 0 | 0 | 3 | 2 |
| `Transfer/64KiB/DecodeString/Unbuffered/New` | 17637 | 17606–18296 | 3716.06 | 32880 | 2 | 4 | 2 |
| `Transfer/64KiB/DecodeString/Unbuffered/Reset` | 16343 | 16321–16378 | 4010.45 | 0 | 0 | 4 | 2 |
| `Transfer/64KiB/DecodeString/Buffered4KiB/New` | 17661 | 17657–17770 | 3711.13 | 32880 | 2 | 3 | 2 |
| `Transfer/64KiB/DecodeString/Buffered4KiB/Reset` | 16468 | 16437–16898 | 3979.81 | 0 | 0 | 3 | 2 |
| `Transfer/8MiB/EncodeBytes/Unbuffered/New` | 2090282 | 2087937–2094037 | 4013.15 | 32848 | 2 | 256 | 257 |
| `Transfer/8MiB/EncodeBytes/Unbuffered/Reset` | 2088052 | 2087785–2092394 | 4017.44 | 0 | 0 | 256 | 257 |
| `Transfer/8MiB/EncodeBytes/Buffered4KiB/New` | 2092893 | 2089405–2156792 | 4008.14 | 32848 | 2 | 256 | 257 |
| `Transfer/8MiB/EncodeBytes/Buffered4KiB/Reset` | 2092397 | 2087397–2380460 | 4009.09 | 0 | 0 | 256 | 257 |
| `Transfer/8MiB/EncodeString/Unbuffered/New` | 2092429 | 2089664–2391758 | 4009.03 | 32848 | 2 | 256 | 257 |
| `Transfer/8MiB/EncodeString/Unbuffered/Reset` | 2092902 | 2088476–2284101 | 4008.12 | 0 | 0 | 256 | 257 |
| `Transfer/8MiB/EncodeString/Buffered4KiB/New` | 2105688 | 2091719–2178195 | 3983.79 | 32848 | 2 | 256 | 257 |
| `Transfer/8MiB/EncodeString/Buffered4KiB/Reset` | 2094357 | 2087658–2235111 | 4005.34 | 0 | 0 | 256 | 257 |
| `Transfer/8MiB/DecodeBytes/Unbuffered/New` | 2092553 | 2088999–2094911 | 4008.79 | 32880 | 2 | 258 | 256 |
| `Transfer/8MiB/DecodeBytes/Unbuffered/Reset` | 2096069 | 2088171–2114428 | 4002.07 | 0 | 0 | 258 | 256 |
| `Transfer/8MiB/DecodeBytes/Buffered4KiB/New` | 2094912 | 2091645–2097737 | 4004.28 | 32880 | 2 | 257 | 256 |
| `Transfer/8MiB/DecodeBytes/Buffered4KiB/Reset` | 2091307 | 2090509–2094675 | 4011.18 | 0 | 0 | 257 | 256 |
| `Transfer/8MiB/DecodeString/Unbuffered/New` | 2090652 | 2089158–2148397 | 4012.44 | 32880 | 2 | 258 | 256 |
| `Transfer/8MiB/DecodeString/Unbuffered/Reset` | 2088059 | 2087006–2098771 | 4017.42 | 0 | 0 | 258 | 256 |
| `Transfer/8MiB/DecodeString/Buffered4KiB/New` | 2094534 | 2091745–2096068 | 4005 | 32880 | 2 | 257 | 256 |
| `Transfer/8MiB/DecodeString/Buffered4KiB/Reset` | 2092368 | 2088070–2101639 | 4009.15 | 0 | 0 | 257 | 256 |

### Owned reads and raw-payload generator

| Case | Median ns/op | Min–max ns/op | Median MB/s | B/op | allocs/op | reads/op | writes/op |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `Owned/1KiB/Bytes` | 353 | 351.7–366.8 | 2909.02 | 1024 | 1 | 3 | 0 |
| `Owned/1KiB/String` | 418.7 | 417.7–447.4 | 2452.57 | 2048 | 2 | 3 | 0 |
| `Owned/64KiB/Bytes` | 18941 | 18793–19356 | 3460.2 | 65536 | 1 | 4 | 0 |
| `Owned/64KiB/String` | 22104 | 21948–24543 | 2965.17 | 131072–131073 | 2 | 4 | 0 |
| `Owned/8MiB/Bytes` | 2237506 | 2235315–2802113 | 3749.09 | 8388615–8388625 | 1 | 258 | 0 |
| `Owned/8MiB/String` | 2424463 | 2416520–2434476 | 3459.99 | 16777228–16777236 | 2 | 258 | 0 |
| `Generator/1KiB` | 269.7 | 269.2–281.8 | 3796.61 | 0 | 0 | 1 | 1 |
| `Generator/64KiB` | 16398 | 16365–16487 | 3996.69 | 0 | 0 | 2 | 2 |
| `Generator/8MiB` | 2087844 | 2086541–2091392 | 4017.83 | 0 | 0 | 256 | 256 |

### Verification and limits

The five memory-check repetitions passed, including the unchanged slice
allocation tests and literal tests for the small generated-source harnesses.
`GOCACHE=/private/tmp/messagepack-go-cache go test ./...` passed in 1.119 s
with authorized localhost-listener access for the existing HTTP tests;
`GOCACHE=/private/tmp/messagepack-go-cache go vet ./...` passed. Formatting,
local Markdown links, and diff checks passed. Production files and public APIs
were unchanged.

These measurements ran only on darwin/arm64. The S09 linux/386 cross-compile
does not become runtime performance or memory evidence for a 32-bit target.
Short samples, default CPU scheduling, process-wide counters, GC activity,
and synthetic endpoints limit comparisons. Repeat with representative readers,
writers, buffering, and longer benchtime when evaluating an application.
