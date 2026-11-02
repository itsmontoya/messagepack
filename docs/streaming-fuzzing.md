# Streaming fuzz and portability checks

S09 verification ran on 2026-11-01 and 2026-11-02 using
`go1.27.2 darwin/arm64` on macOS (Darwin 25.5.0). The module and toolchain were
unchanged. These bounded smoke runs exercise the implemented streaming contract;
they do not establish exhaustive correctness or performance guarantees.

## Targets and independent checks

- `FuzzStreamTypedDecode` exercises all ten typed Decoder operations: nil, bool,
  signed/unsigned integers, both float widths, owned strings/binary, and array/map
  headers. An explicit typed switch compares successful values and consumed
  boundaries with the corresponding slice readers. Float checks preserve raw
  bits, allowing unspecified payload bits only when widening a float32 NaN.
  Failed ordinary results must be zero-valued; successful empty binary is non-nil.
  Owned length limits vary independently of the available input.
- `FuzzStreamPayloadTransfer` exercises both string/binary decode-to-writer and
  encode-from-reader paths. It checks accepted payload counts, original byte
  prefixes, exact successful boundaries, short writes, source/destination error
  identity, simultaneous destination-error precedence, and terminal no-I/O
  behavior. Encoder lengths include zero, exact, longer/shorter than the source,
  arbitrary uint16 lengths, the uint32 maximum, and an oversized length. Oversized
  lengths must fail before I/O without latching a terminal error. A one-byte owned
  result cap demonstrates that decoder transfers use separate limits.
- `FuzzStreamSkip` compares contiguous and scheduled input, then checks successful
  boundaries with a separately bounded recursive slice walk. That oracle uses
  typed slice readers, creates no object tree, and differs from Skip's iterative
  traversal and outstanding-sibling accounting. Complete oracle values also
  predict limit failures from total bytes, value count, and deepest container.
  After success, two copies of the accepted root verify that budgets restart on
  each call and that a following boolean remains intact.
- `FuzzStreamRoundTrip` encodes an explicit nine-element array containing every
  scalar family, owned payloads, and a nested map/array with both transfer forms.
  Exact wire bytes must equal slice appends. Generated integer values and raw
  float bits round-trip through typed calls. Each transfer source includes an
  extra byte that must remain unread. A following boolean and pending EOF verify
  the root boundary. Retained owned results must survive subsequent decodes,
  mutation of the entire wire buffer, copy-buffer reuse, another owned result's
  mutation, and Reset. Encoder reuse must preserve both old and new output.

The shared literal seeds include integer width transitions/extremes, wider integer
forms, float zeros/subnormals/infinities/NaNs, all payload/container header widths,
31/32, 255/256 and 65535/65536 header transitions, signed-32-bit and maximum uint32
lengths/counts, invalid UTF-8/NUL bytes, reserved and every extension tag, nested
last-child containers and container map keys, truncation prefixes, and consecutive
values. Deep array chains include 1, 2, 16, 17, 64, 256, and 257 levels.
Large advertised lengths are backed by tiny inputs. Valid generated payload
lengths stop at 4096 bytes; existing literal/generated unit tests retain coverage
of fully supplied larger header transitions and multi-chunk payloads.

## I/O schedules, state, and bounds

The scheduled reader varies fragments from 1 to 64 bytes using at most 32 control
bytes, or accepts the requested segment when the schedule is empty. It can insert
0–101 consecutive empty reads initially or between progress, and return EOF or a
distinct source error with final bytes or on a later read. Seeds explicitly cover
99/100/101 empty reads. The writer can reject at any selected accepted byte,
return a short count with nil error, or accompany partial/full acceptance with a
distinct error. Both helpers keep counters instead of unbounded I/O logs.

Terminal failures are checked across scalar, owned, header, transfer, and Skip
operations, including a real destination and a subsequent source. Later calls
must return the identical stored error without I/O. Valid/invalid SetLimits calls
cannot clear it. Reset must clear failure/pending EOF, preserve decoder limits,
and isolate the old stream; a nil Reset leaves the codec unattached. EOF returned
with the final complete value must be observed without another underlying read.

Harness bounds are explicit:

| Resource | Bound |
| --- | --- |
| Arbitrary wire/raw payload input | 4096 bytes; larger fuzz inputs are skipped before codec work or harness copies. |
| Fragment/control input | 32 bytes. |
| Each owned result | Typed decode cap 1–4096 bytes; round-trip cap 4096 bytes. |
| Skip limits under mutation | 1–16 container levels, 1–4096 values, 1–4096 wire bytes per call. |
| Independent slice walk | At most 4096 visited values and 33 containing levels; no advertised-count allocation and no loop over children exceeding available tag bytes. |
| Transfer scratch and I/O request | At most 32768 bytes; the reader/writer fail the test immediately for a larger request. |
| Recorded output and write calls | At most 20000 bytes and 20000 calls per writer; the valid round-trip wire fits this ceiling. |
| Scheduled read calls | A hard ceiling of 2000000 per reader; arbitrary-input checks additionally require at most `(input length + 1) * 101 + 102` calls. |
| Consecutive zero-progress reads | Codec must fail at 100, before a 101st empty result; progress resets the count. |

Transfer tests never allocate from a declared wire length: only the input prefix
can reach the bounded destination. Hostile headers cannot bypass the owned cap,
and Skip budgets constrain traversal before large counts are visited. Harness
allocation is bounded by these inputs, fixed codec scratch, small snapshots, and
recorded output. This is not S10 allocation/retained-memory measurement; the Go
fuzz engine's own corpus and worker memory are outside these codec assertions.

The slice API is a value/wire oracle only where contracts match. Slice failures
consume zero bytes and return `ErrTruncated`. Streams may consume prefixes and
return wrapped clean `io.EOF` or `io.ErrUnexpectedEOF`, neither matching
`ErrTruncated`. A complete owned header above its configured cap returns direct
`ErrLimitExceeded` before a missing body can cause EOF, while a slice reader can
report truncation. Skip budget checks can likewise precede missing bytes. Header
success does not require container children. Nonminimal input is never required
to re-encode identically. The fuzz harness preserves these intentional differences
instead of requiring false equivalence.

## Reproduce the timed runs

Each target received its own command with `-fuzztime=30s` and two workers. At most
two commands ran concurrently. Package elapsed time includes baseline processing
and shutdown; execution counts are Go's reported totals, not benchmarks.

```sh
set -o pipefail
GOCACHE=/private/tmp/messagepack-go-cache go test -run '^$' -fuzz '^FuzzStreamTypedDecode$' -fuzztime=30s -parallel=2 ./... 2>&1 | tee /private/tmp/messagepack-s09-FuzzStreamTypedDecode.log
GOCACHE=/private/tmp/messagepack-go-cache go test -run '^$' -fuzz '^FuzzStreamPayloadTransfer$' -fuzztime=30s -parallel=2 ./... 2>&1 | tee /private/tmp/messagepack-s09-FuzzStreamPayloadTransfer.log
GOCACHE=/private/tmp/messagepack-go-cache go test -run '^$' -fuzz '^FuzzStreamSkip$' -fuzztime=30s -parallel=2 ./... 2>&1 | tee /private/tmp/messagepack-s09-FuzzStreamSkip.log
GOCACHE=/private/tmp/messagepack-go-cache go test -run '^$' -fuzz '^FuzzStreamRoundTrip$' -fuzztime=30s -parallel=2 ./... 2>&1 | tee /private/tmp/messagepack-s09-FuzzStreamRoundTrip.log
```

| Target | Date | Final Go elapsed | Package elapsed | Executions | Result |
| --- | --- | --- | --- | ---: | --- |
| `FuzzStreamTypedDecode` | 2026-11-01 | 30 s | 30.422 s | 925477 | Pass |
| `FuzzStreamPayloadTransfer` | 2026-11-01 | 31 s | 31.551 s | 919866 | Pass |
| `FuzzStreamSkip` | 2026-11-02 | 31 s | 31.872 s | 793024 | Pass |
| `FuzzStreamRoundTrip` | 2026-11-02 | 31 s | 31.665 s | 496034 | Pass |

[Recorded run output](streaming-fuzz-results.txt) preserves all progress and final
lines. No failing input or codec defect was found, so there is no failure corpus
or production correction. Literal regression seeds remain in the target files.

## Other validation and interruptions

These commands passed; `-run '^Fuzz'` includes both existing slice targets and
all new streaming seeds:

```sh
gofmt -w stream_fuzz_reader_test.go stream_fuzz_writer_test.go stream_fuzz_test.go stream_decode_fuzz_test.go stream_transfer_fuzz_test.go stream_skip_fuzz_test.go stream_roundtrip_fuzz_test.go
GOCACHE=/private/tmp/messagepack-go-cache go test -run '^Fuzz' -count=1 ./...
GOCACHE=/private/tmp/messagepack-go-cache go test ./...
GOCACHE=/private/tmp/messagepack-go-cache go vet ./...
git diff --check
```

The explicit fuzz-seed check passed in 0.452 s on November 1. The final full suite
passed in 0.656 s on November 2, including existing examples, slice tests, streaming
tests, and fuzz seeds. The full suite uses localhost `httptest` listeners and ran
with sandbox escalation for those listeners. Vet and formatting/diff checks pass.
The Markdown local-link check and staged diff check also pass.

On November 1, automatic approval review could not execute an attempted full test
command because its usage limit was reached; the message explicitly described a
review failure, not an unsafe-action determination. Work resumed on November 2,
and the same authorized full test command passed. That interruption did not
invalidate the two completed fuzz runs, whose logs were retained. No tests or
production logic were changed to bypass the review or sandbox.

## Portability

The following commands passed on November 2 with all new fuzz harnesses present:

```sh
GOCACHE=/private/tmp/messagepack-go-cache GOOS=linux GOARCH=386 CGO_ENABLED=0 go test -c -o /private/tmp/messagepack-s09-linux-386.test
file /private/tmp/messagepack-s09-linux-386.test
```

`file` identifies a statically linked 32-bit Intel 80386 ELF executable. This is
cross-compilation evidence only. Neither ordinary tests nor fuzz seeds were
executed on a 32-bit runtime, and the platform-specific owned-length/int-overflow
test remains unexecuted here.

Read-only probes on November 1 and November 2 found no `qemu-i386` or
`qemu-i386-static`. `qemu-system-i386` exists but cannot directly execute the test
ELF. The Docker status command could not connect to its daemon, and Podman reported
`podman-machine-default* false`:

```sh
command -v qemu-i386 qemu-i386-static qemu-system-i386 || true
docker version --format '{{.Server.Version}}'
podman machine list --format '{{.Name}} {{.Running}}'
```

The Docker/Podman status probes required sandbox escalation. No service or VM was
started, no environment was changed, and no tooling was installed. Future 32-bit
runtime verification remains separate from the successful compile check.
