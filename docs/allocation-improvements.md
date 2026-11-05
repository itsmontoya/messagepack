# Applied allocation improvements

On 2026-11-04, two changes were applied to production code following the
[allocation profiling pass](allocation-profiling.md):

- `Writer.EncodeString` copies strings of one through nine bytes into its existing
  scalar scratch, avoiding a separate 32 KiB payload buffer.
- `Reader.Skip` uses existing scalar scratch to discard payloads of at most nine
  bytes, after checking its byte budget.

Together these add ten lines to the implementation. Both keep the same wire bytes,
I/O boundaries, limits, and terminal error behavior. Larger payloads use the
existing bounded buffer. No public API, shared state layout, or pooling was added.
Short owned string decoding was subsequently optimized in the
[follow-up profiling pass](allocation-followup.md). The broader hook-scratch
refactor remains deferred.

## Before and after

The table below gives medians of five unprofiled samples per version. Allocation
columns report bytes and allocation counts per operation. A negative timing
change means less time per operation. The message is the existing 57-byte,
six-field schema; each of its seven encoded strings fits the existing scratch.

| Case | Before ns/op | After ns/op | Time change | B/op before → after | allocs/op before → after |
| --- | ---: | ---: | ---: | ---: | ---: |
| Message encode, New | 1615 | 115.1 | -92.9% | 32856 → 88 | 3 → 2 |
| Message encode, Reset | 107.3 | 101 | -5.9% | 0 → 0 | 0 → 0 |
| Message encode, buffered New | 1627 | 171.8 | -89.4% | 32856 → 88 | 3 → 2 |
| Message encode, buffered Reset | 162.5 | 155.8 | -4.1% | 0 → 0 | 0 → 0 |
| 1-byte string encode, New | 1522 | 23.04 | -98.5% | 32856 → 88 | 3 → 2 |
| 9-byte string encode, New | 1546 | 22.26 | -98.6% | 32856 → 88 | 3 → 2 |
| 9-byte string encode, Reset | 9.935 | 9.106 | -8.3% | 0 → 0 | 0 → 0 |
| 10-byte string encode, New | 1514 | 1525 | +0.7% | 32856 → 32856 | 3 → 3 |
| 10-byte string encode, Reset | 9.93 | 10.18 | +2.5% | 0 → 0 | 0 → 0 |
| Message Skip, New | 1758 | 290 | -83.5% | 32888 → 120 | 3 → 2 |
| Message Skip, Reset | 268.2 | 265.7 | -0.9% | 0 → 0 | 0 → 0 |
| 9-byte string Skip, New | 1594 | 65.79 | -95.9% | 32888 → 120 | 3 → 2 |
| 9-byte binary Skip, New | 1596 | 75.04 | -95.3% | 32888 → 120 | 3 → 2 |
| 10-byte binary Skip, New | 1596 | 1583 | -0.8% | 32888 → 32888 | 3 → 3 |
| Primitive encode, Reset | 20.83 | 20.77 | -0.3% | 0 → 0 | 0 → 0 |
| Primitive decode, Reset | 79.99 | 79.9 | -0.1% | 0 → 0 | 0 → 0 |

Fresh unbuffered message encoding measured **14.0× faster**, with **99.73% less
allocation by byte count**. Fresh message Skip measured **6.1× faster**, with
**99.64% less allocation by byte count**. Both remove one allocation per fresh
operation. Reused message encoding improved by 5.9%; reused Skip changed by less
than 1% and already reported zero amortized allocation.

The ten-byte cases still allocate the original buffer. Their small timing changes
and the primitive controls give context for measurement variation. The 32 KiB
string path also retains its original allocation cost. Full sample ranges and all
32 cases are in the raw results; these local medians are not throughput guarantees.

The [raw measurements](allocation-improvement-results.txt) contain every sample,
the exact commands, and the order of execution. The historical
[profiling results](allocation-profile-results.txt) remain separate from these
measurements of the applied changes.

## Measurement method

The comparison used Go 1.27.2 on darwin/arm64 with an Apple M3 and the default eight
logical CPUs. Each sample used a 300 ms target. Both test executables used the
same benchmark code, including nine- and ten-byte boundary cases. The before
executable was built before editing either production file; its production code
matches commit `97dd666993514f4eaddbd0b900d61e04234fc91d`. The after executable
contains the two changes above.

Each version ran 32 cases, five times, for 320 samples total. Samples alternated
version order: before/after, after/before, before/after, after/before, before/after.
Runs were sequential, without concurrent tests or profiling. Tables compare
medians; no confidence interval or statistical significance claim is made.

`New` includes fresh codec construction and deliberately escaping codec pointers.
`Reset` reuses a codec. Writer payload scratch is prewarmed outside timing for
Reset cases. Skip includes any initial payload scratch allocation in its timed
batch, so its reported zero B/op on reuse rounds down that one-time cost in the
before version. The optimized short-payload Skip path no longer needs that buffer.

Inputs are small literal fixtures or preconstructed strings. Destinations count
bytes and calls without retaining or copying output. Buffering, when selected,
uses a caller-owned 4 KiB buffer and includes its flush in timing. These are local
synthetic workloads; their timings do not predict file or socket throughput.

The matching binaries, original production files, and run order are retained in
`/private/tmp/messagepack-improvements-20261104` for this session.

## Validation

Validation passed with the applied changes:

- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- Formatting and diff checks.

Tests cover exact wire output at lengths eight, nine, and ten; short writes and
destination errors; fragmented Skip reads and following-value boundaries; every
truncated prefix around the scratch boundary; and allocation budgets for fresh
short-string writers and fresh short-payload readers. Existing ownership, limits,
EOF, payload-transfer, and integration checks also pass. The HTTP integration
tests used authorized localhost-listener access.

## Reproduce

Build the before executable before applying the production change, and the after
executable afterward, keeping benchmark code identical. Run the following cases
with `-benchmem -benchtime=300ms -count=5`:

```sh
go test -run '^$' -bench '^BenchmarkStreamMessage$/^Encode$/^(Unbuffered|Buffered4KiB)$/^(New|Reset)$' -benchmem -benchtime=300ms -count=5
go test -run '^$' -bench '^BenchmarkAllocationStringEncode$/^Bytes(0|1|8|9|10|31|1024|32768)$/^Unbuffered$/^(New|Reset)$' -benchmem -benchtime=300ms -count=5
go test -run '^$' -bench '^BenchmarkAllocationSkip$/^(Message|String9|String10|Binary9|Binary10)$/^Unbuffered$/^(New|Reset)$' -benchmem -benchtime=300ms -count=5
go test -run '^$' -bench '^BenchmarkStreamPrimitives$/^(Encode|Decode)$/^Unbuffered$/^Reset$' -benchmem -benchtime=300ms -count=5
```

For version-order alternation, use the same four patterns with separate compiled
test binaries and `-test.count=1`, following the raw run order. Recreating the
before source requires only restoring reader.go and writer.go from the recorded
commit in a temporary module copy; keep the current benchmark and fixture files.
