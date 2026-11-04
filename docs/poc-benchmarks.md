# Allocation and performance baseline

> Historical slice-PoC measurements with their original ownership and workload
> boundaries. See the [streaming measurement report](streaming-benchmarks.md) for
> stream I/O and memory results, and the [current README](../README.md) for usage.

Measured on 2026-11-01 with `go1.27.2 darwin/arm64`, Darwin 25.5.0,
an Apple M3 (`Mac15,13`), eight logical CPUs, and 24 GiB of memory.
The benchmark process used Go's default CPU setting; result names have the `-8`
suffix. No third-party benchmark tooling or production optimizations were added.

## Reproduce

From the module root:

```sh
GOCACHE=/private/tmp/messagepack-go-cache go test -run '^$' -bench . -benchmem -count=5 ./...
GOCACHE=/private/tmp/messagepack-go-cache go test -run 'TestZeroAllocations|TestOwnedStringAllocations' -v ./...
```

The first command used the default one-second benchmark duration and five samples
per case. Its complete output is retained in
[`poc-benchmark-results.txt`](poc-benchmark-results.txt). The cache override makes
the commands usable in this workspace's filesystem sandbox; it does not alter
codec behavior. The results describe this environment and these inputs, rather
than a timing requirement or a comparison with other implementations.

## Inputs and measurement boundaries

- Primitive inputs are nil, true, signed integer -32769, maximum `uint64`, and
  float32/float64 1.5. Their encoded sizes are 1, 1, 5, 9, 5, and 9 bytes.
  A separate read case widens the five-byte float32 input to float64.
- Array and map headers both use count 65536 and occupy five bytes. They contain
  no children; decoding measures only the header.
- Strings and binary use eight bytes or 65536 bytes of `x`. Encoded string sizes
  are 9 and 65541 bytes; encoded binary sizes are 10 and 65541 bytes.
- The representative 57-byte message is the literal fixture from task 8: a
  six-field map containing ID, name, boolean, float64, binary data, and an array
  with two integers. Its name is an owned string when decoded; its payload is
  borrowed. Schema traversal belongs to the test-only example helpers.

Inputs and reusable destinations are prepared before timing. Each append receives
either a nil destination (`Fresh`) or a slice reset to length zero with enough
capacity for the whole value (`Reused`). Every result is assigned to typed
package-level test sinks, and errors are checked. Owned strings and decoded
messages escape to those sinks, so their copying cost remains observable.

The simple shared benchmark helpers call a function value per operation and check
the returned error. Reported nanoseconds include that harness overhead; they are
baseline figures for the complete measured loop. `MB/s` uses encoded bytes per
operation. For borrowed payloads it describes logical input size, not a payload
copy or scan; header reads likewise process only their encoded header.

Fresh encoding includes ordinary destination growth. Appending a header and then
a payload may grow storage more than once. The large fresh-payload cases exercise
that path without pooling, caching, or preallocation hidden inside the timed loop.

## Allocation regression checks

`TestZeroAllocations` uses `testing.AllocsPerRun(1000, ...)` for four explicit
batches: successful primitive reads, borrowed string/binary reads, array/map
header reads, and appends of every supported family into a 128-byte destination.
All four measured zero heap allocations per run. These tests assert zero only for
those successful inputs and capacity conditions.

`TestOwnedStringAllocations` measures an eight-byte owned string escaping to a
test sink; it recorded one allocation per read. It reports this cost without
enforcing an exact allocation count. Copying is the intended ownership behavior,
so this is a justified exception to the zero-allocation targets. Destination
growth may also allocate. Neither the tests nor the benchmarks promise zero
allocations for every API, payload length, error path, or caller lifetime.

## Five-sample results

All 42 cases completed five samples (210 samples total); the package run took
294.882 seconds and passed. Timing columns summarize those five samples.
Allocation columns were identical across all five samples of each case.

| Case | Median ns/op | Min–max ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: | ---: |
| `AppendPrimitives/Nil/Fresh` | 9.503 | 9.436–9.698 | 8 | 1 |
| `AppendPrimitives/Nil/Reused` | 1.338 | 1.33–1.368 | 0 | 0 |
| `AppendPrimitives/Bool/Fresh` | 9.749 | 9.654–9.864 | 8 | 1 |
| `AppendPrimitives/Bool/Reused` | 1.336 | 1.33–1.34 | 0 | 0 |
| `AppendPrimitives/Int64/Fresh` | 11.5 | 11.44–11.64 | 8 | 1 |
| `AppendPrimitives/Int64/Reused` | 3.179 | 3.166–3.3 | 0 | 0 |
| `AppendPrimitives/Uint64/Fresh` | 20.34 | 20.23–20.45 | 24 | 2 |
| `AppendPrimitives/Uint64/Reused` | 1.605 | 1.597–1.639 | 0 | 0 |
| `AppendPrimitives/Float32/Fresh` | 10.11 | 10–10.28 | 8 | 1 |
| `AppendPrimitives/Float32/Reused` | 1.639 | 1.597–1.681 | 0 | 0 |
| `AppendPrimitives/Float64/Fresh` | 20.67 | 20.46–21.02 | 24 | 2 |
| `AppendPrimitives/Float64/Reused` | 1.602 | 1.598–1.68 | 0 | 0 |
| `ReadPrimitives/Nil` | 1.324 | 1.319–1.326 | 0 | 0 |
| `ReadPrimitives/Bool` | 1.589 | 1.578–1.603 | 0 | 0 |
| `ReadPrimitives/Int64` | 1.872 | 1.869–1.91 | 0 | 0 |
| `ReadPrimitives/Uint64` | 1.617 | 1.608–1.621 | 0 | 0 |
| `ReadPrimitives/Float32` | 1.6 | 1.595–1.608 | 0 | 0 |
| `ReadPrimitives/Float64` | 1.87 | 1.865–1.884 | 0 | 0 |
| `ReadPrimitives/Float64Widen` | 1.632 | 1.606–1.724 | 0 | 0 |
| `Headers/AppendArray/Fresh` | 10.16 | 10.07–10.24 | 8 | 1 |
| `Headers/AppendArray/Reused` | 1.602 | 1.596–1.603 | 0 | 0 |
| `Headers/AppendMap/Fresh` | 10.13 | 10.06–10.24 | 8 | 1 |
| `Headers/AppendMap/Reused` | 1.6 | 1.595–1.632 | 0 | 0 |
| `Headers/ReadArray` | 1.865 | 1.862–1.866 | 0 | 0 |
| `Headers/ReadMap` | 1.87 | 1.863–1.932 | 0 | 0 |
| `Payloads/Small8/AppendString/Fresh` | 23.21 | 22.97–23.7 | 24 | 2 |
| `Payloads/Small8/AppendString/Reused` | 2.94 | 2.926–3.003 | 0 | 0 |
| `Payloads/Small8/AppendBytes/Fresh` | 23.35 | 23.21–23.55 | 24 | 2 |
| `Payloads/Small8/AppendBytes/Reused` | 2.938 | 2.928–3.008 | 0 | 0 |
| `Payloads/Small8/ReadStringOwned` | 10.01 | 9.912–10.39 | 8 | 1 |
| `Payloads/Small8/ReadStringBorrowed` | 2.132 | 2.128–2.139 | 0 | 0 |
| `Payloads/Small8/ReadBytesBorrowed` | 2.134 | 2.128–2.139 | 0 | 0 |
| `Payloads/Large65536/AppendString/Fresh` | 3389 | 3379–3442 | 73736 | 2 |
| `Payloads/Large65536/AppendString/Reused` | 751.3 | 746.9–756.6 | 0 | 0 |
| `Payloads/Large65536/AppendBytes/Fresh` | 3418 | 3397–3507 | 73736 | 2 |
| `Payloads/Large65536/AppendBytes/Reused` | 775.7 | 741.2–806.3 | 0 | 0 |
| `Payloads/Large65536/ReadStringOwned` | 2988 | 2963–3002 | 65536 | 1 |
| `Payloads/Large65536/ReadStringBorrowed` | 2.668 | 2.662–2.67 | 0 | 0 |
| `Payloads/Large65536/ReadBytesBorrowed` | 2.401 | 2.394–2.473 | 0 | 0 |
| `ApplicationMessage/Append/Fresh` | 89.86 | 89.35–90.62 | 120 | 4 |
| `ApplicationMessage/Append/Reused` | 42.64 | 42–43.65 | 0 | 0 |
| `ApplicationMessage/Read` | 58.36 | 57.81–60.76 | 3 | 1 |

Every measured primitive read, borrowed payload read, header read, and reused
append case reported zero bytes and zero allocations per operation. This meets
the scoped targets for these measured paths without production changes.

Fresh nine-byte integer/float encodings and payload encodings allocate twice:
the tag/header starts a destination, then the body exceeds that first capacity.
The representative fresh message grows four times; its reused encoding allocates
zero times. Ordinary append growth is retained for clarity. No correction is
needed when the caller requires fresh storage; callers seeking reuse can supply
sufficient destination capacity.

Owned eight-byte and 65536-byte string reads allocate once, with 8 and 65536 B/op
respectively. The message decoder allocates once for its three-byte owned name
(3 B/op); its binary payload remains borrowed. These copies satisfy the
ownership contract and are intentional exceptions, rather than failed borrowed
read targets.
