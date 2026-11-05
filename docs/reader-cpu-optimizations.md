# Reader CPU fast paths

Applied on November 5, 2026, after the
[CPU profiling investigation](cpu-profiling.md). The implementation adds one
early return and one nil-error guard to reader.go. It adds no state, buffering,
interfaces, or dependencies.

readScalarBody returns immediately when size is one: the preceding tag read
already consumed the entire scalar or container header. This avoids calling
readFull with an empty destination. EOF observed with that tag remains pending
in the shared reader state.

readFull calls errors.Is for EOF only when the underlying error is non-nil.
Successful reads avoid that function call; non-nil errors still follow the
existing wrapped-EOF, partial-read, and terminal-failure handling.

## Before and after

Measured with Go 1.27.2 on darwin/arm64, an Apple M3, and the default eight logical
CPUs. The before build uses reader.go from production commit
`4432a357d5c0bd0adb4cdb749554b4723fd98ee6` through a source overlay. Both binaries
use identical current test and benchmark source; their production source differs
only by these two reader changes.

The run contains 22 cases per version, with five unprofiled 200 ms samples each:
220 samples total. Benchmark groups rotate and reverse, and version order
alternates within each group. Builds, tests, and fuzzing finished before timing.
These are local median timings; ranges and all samples are retained in the
[raw results](reader-cpu-optimization-results.txt).

| Decode workload | Before ns/op | After ns/op | Time change |
| --- | ---: | ---: | ---: |
| Reflected single field | 91.63 | 80.94 | -11.7% |
| Reflected four scalars | 248.5 | 228.5 | -8.0% |
| Reflected mixed struct | 439.9 | 402.5 | -8.5% |
| Reflected nested structs | 242.6 | 217.7 | -10.3% |
| Reflected 1 KiB string | 502.9 | 467.6 | -7.0% |
| Reflected 64 KiB binary | 1190 | 1172 | -1.5% |
| Typed primitive batch, reused | 83.58 | 74.90 | -10.4% |
| Typed message, reused | 328.6 | 295.8 | -10.0% |
| Skip message, reused | 265.7 | 245.0 | -7.8% |
| Owned 8-byte binary | 47.36 | 45.04 | -4.9% |
| Owned 8-byte string | 37.39 | 36.03 | -3.6% |

The separate CPU traversal benchmark also records an 8.1% reduction for its mixed
reflection decoder: 441.9 to 406.0 ns/op. Both mixed benchmarks use the same
six-field shape. The typed streaming message has its existing custom field names
and owns all decoded fields, so it has different allocation and wire costs.

Allocation counts and bytes match before and after in every measured case.
Warmed reflected scalar and nested decoding still allocate zero. Reflected mixed
and reused binary decoding retain their one owned-name allocation; typed message
decoding retains its existing eight owned-result allocations.

The nine encoding controls range from -2.1% to +0.5% in median time. The code
change affects reading, and those small encoding differences are not claimed as
an improvement. The 64 KiB decode timing ranges overlap; copying dominates that
case, so the small median reduction does not establish a consistent speedup.
No statistical significance or cross-platform performance claim is made.

## Validation

The existing Reader EOF table now also covers clean and wrapped EOF with no
bytes, complete tag-only integers delivered with EOF, and wrapped EOF during
or after wider integers. It checks results, truncation classification, operation
context, pending EOF, and the absence of further reads after terminal failures.
Existing tests cover fragmentation, no-progress handling, cached tags, limits,
ownership, and allocation expectations.

Passed:

- go test ./... and go test -race ./...; the existing HTTP integration tests used
  previously authorized localhost listener access.
- go vet ./... and the focused Reader/Decoder tests.
- FuzzStreamTypedDecode for ten seconds with two workers: 207556 executions.
- FuzzDecoderReflection for ten seconds with two workers: 300491 executions.
- All 220 benchmark samples, including their output and decoded-value checks.
- gofmt and diff checks.

## Reproduce

The raw results include commands, source and binary hashes, the overlay, the
exact reader/test diff, and the comparison runner. Matching executables and the
original reader source remain available for this session at:

```text
/private/tmp/messagepack-reader-fast-paths-c_ac4fp8
```

Rebuild both executables with the same benchmark source, supplying the preserved
reader.go through before-overlay.json only for the before build. The recorded
runner then repeats the alternating comparison:

```sh
export GOCACHE=/private/tmp/messagepack-go-build
profile_dir=/private/tmp/messagepack-reader-fast-paths-c_ac4fp8
go test -overlay "$profile_dir/before-overlay.json" -c -o "$profile_dir/before.test" .
go test -c -o "$profile_dir/after.test" .
python3 "$profile_dir/run.py"
```

For an independent current measurement, run:

```sh
GOCACHE=/private/tmp/messagepack-go-build go test -run '^$' \
  -bench '^BenchmarkReflectionStruct$/./^Decode$' \
  -benchmem -benchtime=200ms -count=5
```
