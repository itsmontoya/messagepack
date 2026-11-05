# Cached reflection allocations

The [decoder and string follow-up](decoder-string-optimizations.md) records the
subsequent decoder-context reuse, cached child dispatch, and string staging changes.
The measurements below preserve the earlier implementation stages.

Measured on 2026-11-04 with Go 1.27.2, darwin/arm64, and an Apple M3. This initial
comparison measures the uncached fallback and cached reflection plans before
the byte-slice reuse follow-up below. Both versions use the same
[struct benchmarks](../reflection_benchmark_test.go).

Each case has three unprofiled samples with a 300 ms target. Allocation counts
and bytes were identical across those samples. Codecs and struct pointers are
reused; construction, fixture encoding, and a warm-up operation are excluded.
Encoding writes to a counting sink, while decoding replays a small wire fixture
into an existing target. These figures measure allocated bytes per operation,
not retained memory or network/storage performance.

| Struct | Encode B/op before → after | Encode allocs/op before → after | Decode B/op before → after | Decode allocs/op before → after |
| --- | ---: | ---: | ---: | ---: |
| One integer field | 296 → 24 | 5 → 1 | 360 → 48 | 8 → 1 |
| Four scalar fields | 344 → 24 | 8 → 1 | 520 → 48 | 20 → 1 |
| Six mixed fields | 456 → 24 | 12 → 1 | 720 → 54 | 32 → 3 |
| Nested structs | 832 → 24 | 12 → 1 | 1008 → 48 | 24 → 1 |

The scalar case has uint64, int64, bool, and float64 fields. The mixed case has
uint64, a three-byte string, bool, float64, a two-byte binary slice, and a
two-element int64 array. The nested case has two child structs, each with one
scalar field. Encoding receives pointers prepared before timing, so the results
exclude caller-side boxing of struct values.

## What changed

Plans are keyed by reflect.Type and shared across Encoder and Decoder instances.
Each plan selects its codec functions, checks value and pointer hook eligibility,
and records exported struct fields once. Cache publication uses sync.Map and
sync.Once. Child plans initialize during traversal to support recursive types
without recursively locking initialization. Warm cache lookups allocate zero.

The initial scalar profiles, sampling every allocation over 20000 iterations,
attribute 256 B per struct to rebuilding field metadata. They also show boxing
through reflect.Value.Interface during unsuccessful hook checks. Cached hook
eligibility removes those unsuccessful assertions and their boxed copies.

Struct keys use the typed string reader directly, then look up the field in the
cached map. In these short-key workloads, the compiler avoids allocating the
temporary lookup string. Longer keys can still allocate staging storage through
the Reader's owned payload path.

At this initial stage, per-call reflection codec state allocates: 24 B for encoding
and 48 B for decoding on this toolchain. The mixed decoder additionally allocates
owned storage for its string and binary result. General slices, maps, new pointers,
float32 conversions, and other schemas can incur further allocations. These
results do not imply that every warmed struct or decoded field allocates zero.

Plans retain type metadata and codec functions for the process lifetime. They
do not retain stream state or application values. The cache can be shared by
independent codecs; individual codecs still require synchronization for shared use.

## Destination byte-slice reuse

Measured on 2026-11-05 with the same toolchain, fixtures, and three sequential
300 ms samples. Reflected byte-slice fields now reuse their destination backing
arrays when capacity is sufficient. Payload limits still apply before any write.

| Mixed struct decode | B/op before → after | allocs/op before → after |
| --- | ---: | ---: |
| Retain destination fields between calls | 54 → 51 | 3 → 2 |
| Clear destination fields before each call | 54 → 54 | 3 → 3 |

The remaining allocations in the reused mixed case are 48 B of per-call codec
state and the three-byte owned string. Scalar and nested struct allocation
counts remain unchanged. Tests for two-byte, 1 KiB, and 64 KiB reused binary
fields allow at most the one per-call codec allocation.

Empty binary preserves available destination capacity, while wire nil clears the
slice. Growing beyond capacity allocates new storage. Decoding into the same
target may overwrite earlier aliases, including a partial prefix on failure.
Typed Reader.DecodeBytes still returns fresh owned results.

The isolated tinylib/msgp v1.6.5 comparison was rerun for the mixed fixture with
identical wire data and warmed codecs. With a retained destination, our decoder
uses 51 B / 2 allocations versus tinylib's 3 B / 1 allocation. Clearing the
destination yields 54 B / 3 allocations versus 6 B / 2 allocations. Both modes
now differ by our fixed 48 B / one-allocation codec state. The temporary harness
and raw before/after logs are retained in
`/private/tmp/messagepack-tinylib-alloc.izWhKC` for this session.

## Reused encoding context, child handlers, and prefixes

Measured on 2026-11-05 with the same toolchain and fixtures. These figures are
medians of five sequential, unprofiled 300 ms samples for each implementation.
Each case reuses its Encoder, a prepared struct pointer, and a counting output;
construction and warm-up are excluded. The comparison uses generated
tinylib/msgp v1.6.5 streaming encoders, including their Flush call, against our
unbuffered Encoder. Both implementations produce identical wire data.

| Struct | Our encode ns/op before → after | Our B/op before → after | Our allocs/op before → after | Tinylib ns/op | Tinylib B/op / allocs/op |
| --- | ---: | ---: | ---: | ---: | ---: |
| Four scalar fields | 129.8 → 68.49 | 24 → 0 | 1 → 0 | 19.82 | 0 / 0 |
| Six mixed fields | 225.5 → 127.1 | 24 → 0 | 1 → 0 | 41.15 | 0 / 0 |
| Nested structs | 160.1 → 67.22 | 24 → 0 | 1 → 0 | 16.24 | 0 / 0 |

Our encoding is 1.8–2.4 times faster than the previous cached-reflection path in
these samples. Both libraries now allocate zero for these warmed encodes;
tinylib remains 3.1–4.1 times faster here. This measures the actual streaming APIs,
including their different buffering behavior, rather than isolating field access.

The Encoder now owns and reuses its reflection context and Writer scratch instead
of allocating a context for every call. Plans link statically known fields,
pointer targets, array/slice elements, and map keys/values directly to cached
child plans. Each child still initializes lazily through sync.Once, so recursive
types do not deadlock. Dynamic interface values look up their actual types.
Field access continues through reflection; there is no generator or build step.

Struct plans also retain encoded map headers and field-name prefixes. The map
header is combined with the first key; later prefixes hold one key each. Cached
fragments use the existing Writer failure handling and at most 32 KiB per output
request. They preserve the wire format and immediate streaming behavior without
adding an output buffer or changing Close semantics.

Addressable float32 fields, including named types, use a typed reflected pointer
to avoid boxing while preserving their bits. The zero-allocation regression
tests cover these fields as well as scalar, mixed, and nested structs. Passing
structs by value, user hooks, map traversal, and other schemas can still allocate;
these results apply to the measured warmed pointer-input cases. Decoding's
per-call context allocation remains unchanged in this encoding-focused change.

The temporary comparison harness is
`/private/tmp/messagepack-tinylib-alloc.izWhKC`; its current binary, raw before/after
samples, and median CSVs for this phase are retained in
`/private/tmp/messagepack-encoding-mechanisms.8mhCUW` for this session.

## Reproduce

```sh
GOCACHE=/private/tmp/messagepack-reflection-go-cache go test \
  -run '^$' -bench '^BenchmarkReflectionStruct$' \
  -benchmem -benchtime=300ms -count=3
```

Allocation regression tests cover zero-allocation warm cache lookups and warmed
struct encoding, plus at most one allocation for reused scalar-only decoding. Concurrent
first use of a recursive struct, named-type separation, hook addressability,
and dynamic interface hooks have explicit tests.
Byte-slice reuse tests cover named element types, growth, length changes, empty
and nil payloads, retained aliases, input independence, limits, and partial reads.
Encoding tests cover empty structs, map/key length boundaries, bounded cached
prefix writes, failures before field dispatch, and reuse across changing values
and types. The encoding fuzzer checks repeated round trips and exact float bits.
