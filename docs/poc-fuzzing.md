# Fuzz and portability checks

> Historical slice-PoC verification. These recorded runs and their dates are
> preserved. See the [streaming fuzz report](streaming-fuzzing.md) for stream I/O
> and Skip checks, and the [current README](../README.md) for usage.

Task 9 verification on 2026-11-01 used `go1.27.2 darwin/arm64`.
These are smoke runs, not a guarantee that every malformed input has been explored.

## Targets and invariants

`read_fuzz_test.go` has one arbitrary-input target for each of the eleven read
APIs. Shared literal seeds include every leading tag, integer boundaries and
nonminimal encodings, float special values, string/binary families, container
count boundaries, and lengths around the signed 32-bit limit and maximum wire
length. Every prefix of each fixture is seeded, as is a complete fixture with
trailing bytes. Unit tests retain the independent expected wire/value fixtures.

Every read target checks unchanged input, a consumed count between one and the
input length on success, and zero consumption on failure. Typed targets also
check the zero failure value; slice failures must be nil, and successful borrowed
views must have capacity equal to length. Panics fail the target normally.

`roundtrip_fuzz_test.go` generates signed/unsigned integers, raw float32/float64
bits, strings, and binary payloads. It checks decoded values and complete
consumption. Float comparisons use raw bits, including NaNs and signed zero.
String/binary generated payloads above 4,096 bytes are skipped before encoding to
bound harness allocations. The arbitrary-input readers remain unbounded by this
payload cap. No test requires nonminimal input to re-encode to identical bytes.

The payload readers were also reviewed: they compare advertised size with bytes
actually present before conversion to `int`, offset addition, or slicing.
`ReadString` copies only a fully present payload. Header readers neither allocate
containers nor traverse children. No production correction was needed.

## Reproduce the smoke runs

Run each listed target separately, replacing `TARGET` with its exact name:

```sh
GOCACHE=/private/tmp/messagepack-go-cache go test -run '^$' -fuzz '^TARGET$' -fuzztime=30s -parallel=2 ./...
```

The recorded runs used at most two processes concurrently, each with two fuzz
workers. Each target received its own 30-second run; package elapsed time includes
startup, baseline corpus processing, and shutdown. Execution counts are totals
reported by Go, not performance benchmarks.

| Target | Go-reported elapsed | Package elapsed | Executions | Result |
| --- | --- | --- | ---: | --- |
| `FuzzReadNil` | 30 s | 30.520 s | 4,404,475 | Pass |
| `FuzzReadBool` | 30 s | 30.285 s | 4,374,413 | Pass |
| `FuzzReadInt64` | 30 s | 30.266 s | 4,339,397 | Pass |
| `FuzzReadUint64` | 30 s | 30.371 s | 4,310,053 | Pass |
| `FuzzReadFloat32` | 30 s | 30.307 s | 4,276,240 | Pass |
| `FuzzReadFloat64` | 30 s | 30.371 s | 4,288,333 | Pass |
| `FuzzReadString` | 30 s | 30.369 s | 4,228,611 | Pass |
| `FuzzReadStringBytes` | 30 s | 30.231 s | 4,243,840 | Pass |
| `FuzzReadBytes` | 30 s | 30.233 s | 4,163,268 | Pass |
| `FuzzReadArrayHeader` | 30 s | 30.398 s | 4,188,422 | Pass |
| `FuzzReadMapHeader` | 30 s | 30.252 s | 4,136,192 | Pass |
| `FuzzRoundTripInt64` | 30 s | 30.455 s | 4,262,244 | Pass |
| `FuzzRoundTripUint64` | 30 s | 30.431 s | 4,294,066 | Pass |
| `FuzzRoundTripFloat32` | 30 s | 30.273 s | 4,336,439 | Pass |
| `FuzzRoundTripFloat64` | 30 s | 30.431 s | 4,196,134 | Pass |
| `FuzzRoundTripString` | 31 s | 31.211 s | 104,322 | Pass |
| `FuzzRoundTripBytes` | 31 s | 31.214 s | 66,583 | Pass |

No failing input was found, so no failure corpus or production fix was added.
`gofmt`, `go test ./...`, `go vet ./...`, and `git diff --check` passed; Go commands
used `GOCACHE=/private/tmp/messagepack-go-cache`.

## Portability

This command compiled all tests, including the fuzz seed harnesses, successfully:

```sh
GOCACHE=/private/tmp/messagepack-go-cache GOOS=linux GOARCH=386 CGO_ENABLED=0 go test -c -o /private/tmp/messagepack-linux-386.test
file /private/tmp/messagepack-linux-386.test
```

`file` identified a statically linked 32-bit Intel 80386 ELF executable. This is
compilation evidence only; the 32-bit tests were not executed. The host is
darwin/arm64, no `qemu-i386` userspace emulator was found, and probing the existing
Docker daemon failed because its socket is inaccessible in the sandbox.
System-mode QEMU is installed but does not directly execute this ELF binary; no
VM was booted and no images or tooling were installed.
