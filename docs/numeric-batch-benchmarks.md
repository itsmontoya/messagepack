# Numeric batch decoding baseline

Measured on November 5, 2026 with Go 1.27.2, darwin/arm64, Apple M3, and
eight logical CPUs. These measurements precede the optional SIMD implementation
and establish its scalar baseline. See the [SIMD follow-up](numeric-simd-benchmarks.md)
for current optimized paths and their separate measurements.

```sh
go test -run '^$' -bench '^BenchmarkReader(Int64s|Uint64s|Float64s)$' -benchmem -benchtime=100ms -count=3
```

The measurement used a writable `GOCACHE` in `/private/tmp`; no other tests or
benchmarks ran concurrently. All 48 cases completed three samples and passed.

## Inputs and measurement boundaries

- Signed inputs cycle through -1, -129, and minimum int64, using one-, three-,
  and nine-byte encodings.
- Unsigned inputs cycle through 7, maximum uint64, and 1 encoded as int16,
  using one-, nine-, and three-byte encodings.
- Float inputs alternate 1.5 encoded as float64 and 2.5 encoded as float32,
  exercising both native float64 and widening.

Each scalar/batch pair uses identical prepared bytes and reuses a Reader and
destination slice. Timed loops reset a bytes.Reader, decode the requested count,
and check errors and completion counts. Final values and consumed input are
validated outside timing. Setup and destination allocation are excluded.
There are no container headers, files, network calls, or reflection in these cases.

## Results

Times are median nanoseconds per complete batch, including the scalar loop in
the scalar columns. Every sample reported **0 B/op and 0 allocs/op**.

| Elements | Int64 scalar | Int64 batch | Uint64 scalar | Uint64 batch | Float64 scalar | Float64 batch |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 1 | 10.62 | 12.06 | 11.06 | 12.36 | 14.54 | 15.45 |
| 2 | 29.27 | 30.3 | 28.9 | 29.65 | 29.82 | 30.79 |
| 4 | 60.89 | 61.24 | 60.36 | 59.92 | 59.42 | 59.43 |
| 8 | 129.2 | 129.7 | 128.1 | 126.7 | 121.6 | 118.5 |
| 16 | 259 | 253.2 | 249.1 | 245 | 234.1 | 231.5 |
| 64 | 1031 | 1005 | 984.4 | 978.5 | 910.9 | 900.3 |
| 256 | 4086 | 3990 | 3913 | 3885 | 3638 | 3588 |
| 1024 | 16126 | 15873 | 15869 | 15463 | 14530 | 14397 |

The scalar batch wrappers add a small cost for short runs and remain close to
the explicit scalar loops for longer runs. These short samples do not establish
a meaningful speedup or a SIMD threshold. Both paths still perform the same
per-element stream reads. Future implementations should compare against both
columns and preserve the documented conversion, partial-result, and EOF behavior.
