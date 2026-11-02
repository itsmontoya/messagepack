# Streaming-first MessagePack

Build the package around an `Encoder` writing to `io.Writer` and a `Decoder`
reading from `io.Reader`. Primary use cases are decoding an HTTP response body,
processing consecutive messages, and writing large objects to files without
holding an entire encoded message or payload in memory. Keep the implementation
small, typed, explicit, and standard-library-only.

The S01 streaming contract is approved; implementation tasks remain pending.
The completed PoC plan and its handoffs are preserved in
[docs/poc-todo.md](docs/poc-todo.md).

## Instructions for each fresh agent

Read this entire file, [AGENTS.md](AGENTS.md), [STYLEGUIDE.md](STYLEGUIDE.md), the
current source, and the preceding tasks' handoffs. Execute S01–S11 in order, one
task per agent; each task depends on all earlier tasks being complete. The shared
requirements below are part of every task. Do not rely on prior chat history.

**Authorship boundary:** `AGENTS.md` reserves production code and architectural
decisions for humans. On 2026-11-01 the maintainer approved the reviewed streaming
contract and authorized agent production work for S01–S11, as recorded with the
actual reply and its question in [the streaming contract](docs/streaming-contract.md).
That session-specific authorization covers only these streaming tasks; the
standing policy and style guide remain unchanged. The old authorization for PoC
tasks 1–11 is separate. Do not broaden this authority or mark an implementation
task complete when only its proposal or tests exist.

For each task:

- Inspect the current checkout and preserve unrelated work. Use the existing module
  and Go version in `go.mod`; a toolchain or dependency change is outside this plan.
- Add tests and exported GoDoc with the task, not only at the end. Use literal wire
  fixtures as well as round trips. Follow the style guide: one file per primary
  type, all methods in that file, constructor immediately above the type, named
  results where appropriate, explicit returns, early exits, grouped stateful
  variables, and no shadowing. Keep test helpers explicit and small.
- Keep `Encoder` methods in `encoder.go` and `Decoder` methods in `decoder.go`.
  Any additional primary type needs its own file; do not scatter methods across
  files by wire family. Test cases may be separated by feature.
- Run `gofmt` on changed Go files, `go test ./...`, `go vet ./...`, and
  `git diff --check` for implementation/test changes. Documentation-only work needs
  link and diff checks. Record failures and unavailable checks honestly.
- Update the task checkbox only when its acceptance criteria pass. Append a
  **Handoff** under that task with changed files, exact public API and contract
  decisions, commands/results, remaining limitations, and what the next agent needs.
- Resolve contract conflicts in the written contract before dependent work proceeds.
  Never silently change previously established behavior or broaden authorization.
- If preparing a PR, follow the template required by `AGENTS.md`. At planning time
  `.github/PULL_REQUEST_TEMPLATE.md` is absent; report that rather than inventing
  its required structure.

## Current repository context

- `nil.go`, `bool.go`, `integer.go`, and `float.go` implement typed slice appends
  and reads. `string.go` and `bytes.go` include private payload-header helpers;
  their public readers require the complete payload in memory.
- `array.go` and `map.go` handle headers only. `errors.go` defines five sentinels
  and tag classification. No Encoder, Decoder, stream state, or I/O API exists.
- [docs/poc-contract.md](docs/poc-contract.md) describes current wire, conversion,
  slice ownership, and error behavior. It is the compatibility baseline, not a
  streaming contract. In particular, failed slice reads consume zero bytes;
  arbitrary readers cannot provide that rollback guarantee.
- Per-family tests, `read_fuzz_test.go`, `roundtrip_fuzz_test.go`,
  `allocation_test.go`, and `benchmark_test.go` cover the slice codec.
  `application_message_test.go`, `integration_test.go`, and `example_test.go`
  demonstrate a six-field map with a nested array and a binary payload.
- Keep the existing exported slice API and its tests compatible in this phase.
  It can remain a low-level building block and wire oracle. Make streams the
  primary documented workflow. Do not implement streaming by accumulating a whole
  value into a buffer and calling the slice API.

## Shared streaming requirements

S01 resolved these requirements in the approved streaming contract. Later agents
use that contract rather than choosing their own method names or semantics.

### API and supported values

- Concrete `Encoder` and `Decoder`, constructed from `io.Writer` and `io.Reader`.
  Use typed operations for nil, bool, int64, uint64, float32, float64, strings,
  binary, and array/map headers. Names include `EncodeBool`, `DecodeBool`,
  and equivalent names for the other families. All encoding methods can fail.
- Provide both convenient in-memory string/binary operations and bounded-memory
  transfers. Transfer names are `EncodeBytesFrom`, `EncodeStringFrom`,
  `DecodeBytesTo`, and `DecodeStringTo`. Encoders take a known byte length and a
  source reader; decoders parse the wire length and copy to a destination writer.
  Specify transferred-payload counts on partial failure, separately from headers.
- Preserve existing integer width selection, range checks, float bit behavior,
  string/binary distinction, byte-preserving strings, and nil semantics.
  Reserved tags and extensions retain their existing classification; extensions
  and timestamps remain unsupported in this phase, including during skipping.
- Array counts mean elements and map counts mean pairs. Callers traverse children
  incrementally and own schema validation. Headers alone do not validate children.
  Typed calls do not track a whole application object's completeness.
- Include bounded `Skip` of one complete supported value for unknown fields,
  including nested arrays/maps. It must leave the next value available.
- Generic reflection-based `Encode(any)`/`Decode(any)`, struct tags, automatic
  mapping, extension support, code generation, pooling, and concurrent use of a
  single codec remain outside this phase. Application objects use explicit typed
  calls; no generic dispatch framework is needed to demonstrate the use cases.

### Memory, buffering, and lengths

- Codec working memory on transfer paths must be independent of payload length
  and total object size. Fixed-size scalar/header scratch and bounded copy buffers
  are acceptable. There must be no `io.ReadAll` or whole-message staging in these
  paths, including through convenience fast paths or helper interfaces.
- MessagePack requires payload lengths and container counts before their contents.
  Unknown-length input requires an application-level sequence/chunk schema or
  external staging/counting. Do not silently spool, seek back, invent indefinite
  encodings, or buffer an unknown-length object inside the codec.
- A single string/binary length and container count fit the existing uint32 wire
  limit. Stream larger datasets as multiple values or suitable containers, with
  each count/length valid. Use wide arithmetic for counters and map child counts;
  check before narrowing to `int`, especially on 32-bit platforms.
- Recommend no hidden read-ahead or output buffering initially. Callers may supply
  `bufio.Reader`/`bufio.Writer`; those wrappers must remain in use for subsequent
  access. The codec must not flush or close caller-owned streams. A successful
  encode means its writer accepted the bytes, not that a file is durable.
- Owned decode results must remain stable after the next call or stream reuse.
  Do not expose borrowed views into a Decoder's reusable scratch storage.
  Convenient materializing reads require an explicit, finite size policy checked
  before allocation. Transfer APIs must not inherit a small materialization cap.
- Define finite skip depth/work limits and caller controls. Reject hostile lengths
  before allocating; do not allocate containers from advertised counts. Explain
  how callers bound response bytes and apply deadlines/cancellation through their
  own readers, transports, or contexts. Do not add hidden background goroutines.

### I/O and failure semantics

- Read exactly the bytes needed for the current operation. Handle fragmented reads
  and readers returning bytes together with an error. A final `(n, io.EOF)` that
  supplies all requested bytes must not falsely truncate the value. Specify when
  a simultaneous non-EOF error is observed. Bound repeated zero-progress reads.
- Return `io.EOF` when no byte of the next requested value is available. Report
  `io.ErrUnexpectedEOF` for an incomplete header or payload. Decide explicitly
  whether that error also matches `ErrTruncated`; retain old slice semantics.
  When callers have already started a container/schema, they must interpret EOF
  between required child calls as an incomplete application message, not success.
- Treat a short write with nil error as `io.ErrShortWrite`. Honor partial writes
  with errors, stop emitting on failure, and do not retry a whole value after a
  prefix has escaped. Check argument/length errors before emitting its header.
- Recommend sticky terminal errors after stream-consuming decode failures or
  output failures: later operations perform no more I/O and report the stored
  failure. Specify EOF, pre-I/O validation errors, reset, and all retry exceptions
  explicitly. No rollback or automatic resynchronization is promised.
- Wrap external I/O errors with operation context using `%w`, preserving
  `errors.Is` identity. Return descriptive codec/state sentinels directly as the
  style guide requires. Distinguish malformed data, unsupported types, overflow,
  configured limits, source failures, and destination failures.
- Define zero-value behavior, nil interface handling, `Reset` semantics if exposed,
  and ownership. Reset cannot repair a partially written message or rewind a
  partially consumed source. Do not imply codecs are safe for concurrent use.

## S01 — Establish the streaming contract

- [x] Complete S01.

**Read:** this file, `AGENTS.md`, `STYLEGUIDE.md`, `go.mod`,
`docs/poc-contract.md`, `errors.go`, and all existing exported functions.

**Deliverables:** `docs/streaming-contract.md` with exact public
signatures, method placement, ownership rules, error/state transition tables,
limits/defaults, and the compatibility strategy. Clearly label approval and
implementation authorization status; approval must be recorded when it occurs.

**Work:**

- Resolve every shared requirement above, including concrete payload method
  signatures/count types, materialization limits and how callers configure them,
  skip limits, EOF matching, sticky error policy, and reset lifecycle.
- Choose the smallest surface that supports both small typed values and huge
  streamed payloads. Specify uninitialized/nil behavior and whether `Reset` is
  present. Keep existing slice APIs unchanged.
- Document fixed-length source behavior: consume exactly the declared payload,
  fail on early EOF, and leave extra source bytes unread. Define zero-length
  transfers and partial-progress reporting in both directions.
- Record an error matrix covering empty input, truncated tag/body, mismatched
  family, numeric overflow, limits, and source/destination I/O failures. Separate
  stream consumption from the old slice API's zero-consumption guarantee.
- Describe the HTTP-body and file-writing workflows and which task proves each.
  Confirm the wire choices against the official specification linked in the PoC
  contract; record any compatibility issue before implementation.

**Acceptance:** no unresolved API/lifecycle/limit choices remain for downstream
agents; a maintainer has settled architectural decisions and the applicable
production-work authority is recorded before S02 begins. Documentation preparation
alone does not imply that approval or authorization has occurred.

**Historical handoff (documentation proposal, 2026-11-01; superseded by approval below):**

- Added [the proposed streaming contract](docs/streaming-contract.md); changed
  files are `docs/streaming-contract.md` and this handoff in `TODO.md`. No
  production code, tests, dependencies, `AGENTS.md`, or `STYLEGUIDE.md` changed.
- Proposed exact Encoder/Decoder typed methods, reset, `DecoderLimits` and
  `SetLimits`, uint64 transfer progress, owned decode results, and direct
  `ErrUninitialized`/`ErrInvalidArgument`/`ErrLimitExceeded` sentinels. Proposed
  defaults are a 16 MiB materialization cap and Skip limits of 64 container levels,
  1048576 values, and 64 MiB wire bytes, with a hard depth ceiling of 256.
- Specified 32 KiB bounded payload transfers, fixed-length source behavior,
  destination-accepted progress, simultaneous I/O-error precedence, pending EOF,
  sticky failures, retry/reset boundaries, checked Skip accounting, nil-interface
  handling, caller-owned buffering/lifetimes, and HTTP/file proof obligations.
- Checked proposed wire choices against the official MessagePack specification
  linked in the contract on 2026-11-01. No wire compatibility issue was identified;
  existing slice APIs and their distinct ownership/error behavior remain the
  compatibility baseline.
- Validation: `git diff --check` and `git diff --cached --check` pass. The
  following local-link check passes with `8 local links: PASS`; opening the linked
  official specification also succeeded. Go tests/vet were not rerun for this
  documentation-only task, and no implementation behavior is claimed.

  ```sh
  python3 -c 'from pathlib import Path; import re; p=[Path("TODO.md"),Path("docs/streaming-contract.md")]; links=[f.parent/t for f in p for t in re.findall(r"\[[^\]]+\]\(([^)]+)\)",f.read_text()) if "://" not in t]; assert all(f.exists() for f in links); print(len(links),"local links: PASS")'
  ```

- **At this draft handoff, S01 remained incomplete.** The contract was unapproved,
  and no explicit production-work authorization for S01–S11 had been recorded.
  The maintainer still needed to settle the proposed architectural choices and
  record applicable authority before S01 could be checked or S02 could begin.
  Approval did not follow from the earlier PoC authorization or the draft commit.

**Handoff (approved S01 completion, 2026-11-01):**

- Recorded the maintainer's “Please continue” reply to the explicit question
  asking for contract approval and production-code authorization for S01–S11.
  The streaming contract now records the actual exchange, scope, and date.
  `AGENTS.md` and `STYLEGUIDE.md` remain unchanged; unrelated work is not authorized.
- Recorded approval of the exact public API, ownership, limits, I/O/error
  precedence, reset, and Skip accounting without behavioral revisions. Updated
  current approval/authorship wording and checked S01. Changed files are
  `docs/streaming-contract.md` and `TODO.md`; implementation is still pending.
- Validation: the local-link `python3 -c` command above reports
  `10 local links: PASS`;
  `git diff --check` and `git diff --cached --check` pass. No Go files changed,
  so Go tests/vet were not rerun for this documentation-only completion.
- No unresolved API, lifecycle, limit, or authorization choice blocks S02. The
  next fresh agent must implement only the S02 Encoder foundation/scalar surface,
  with tests and GoDoc under the approved contract, then validate and commit its
  own handoff before S03 begins. S02–S11 remain unchecked until their criteria pass.

## S02 — Encoder foundation and scalar writes

- [x] Complete S02.

**Read:** S01 contract; `nil.go`, `bool.go`, `integer.go`, `float.go`, their tests,
and `errors.go`. **Deliverables:** `encoder.go`, scalar/I/O tests, and required
error documentation under the established authority.

**Work:** implement construction, bounded scratch storage, write/error lifecycle,
and typed nil, bool, integer, and float operations. Reuse existing small scalar
encoding helpers where clear; never grow scratch with total output size. Add the
chosen reset/uninitialized behavior now. Do not add payload or container methods
before their tasks.

**Acceptance:** literal fixtures cover all integer transitions, signed/unsigned
extremes, float widths, signed zero, infinities, and NaNs. Consecutive operations
produce consecutive values. Fault writers cover errors before output, partial
writes with errors, short writes with nil error, and zero progress. Verify stored
failure behavior and that the writer is never closed/flushed by the Encoder.

**Handoff (2026-11-01):**

- Added `encoder.go`, `encoder_test.go`, and `encoder_test_writer_test.go`;
  extended `errors.go` with documented `ErrUninitialized`; updated this task.
  Production work uses the S01–S11 authority recorded in the approved contract.
- Public API added: `NewEncoder(w io.Writer) (e *Encoder)`, `Encoder`,
  `(*Encoder).Reset(w io.Writer)`, and `EncodeNil`, `EncodeBool(bool)`,
  `EncodeInt64(int64)`, `EncodeUint64(uint64)`, `EncodeFloat32(float32)`, and
  `EncodeFloat64(float64)`, each returning `(err error)`.
- Scalars reuse existing append primitives with fixed nine-byte scratch. There
  is no accumulated output, hidden buffering, retry, flush, or close. The private
  write helper retains destination-accepted counts even on failure for S04's
  future transfer paths. Short nil-error writes become wrapped `io.ErrShortWrite`;
  every writer error, including a full-count error, is contextual and terminal.
  Later calls return the identical stored error without I/O. Unattached calls
  return direct `ErrUninitialized`; reset clears errors and attaches its argument
  without repairing the previous stream. No contract changes were required.
- Literal fixtures cover every integer-width transition, extrema, float widths,
  signed zero, subnormals, infinities, and NaN payload bits. Tests also check
  immediate consecutive output, partial/full/error/zero writes, sticky errors
  across all scalar methods, zero/nil/typed-nil initialization, reset isolation,
  and caller-owned writer lifetimes. Existing slice tests remain unchanged.
- Validation passed: `gofmt -w encoder.go errors.go encoder_test.go
  encoder_test_writer_test.go`; `GOCACHE=/private/tmp/messagepack-go-cache go test
  ./...`; `GOCACHE=/private/tmp/messagepack-go-cache go vet ./...`;
  `git diff --check`; and `git diff --cached --check`. Reviewed the staged changes.
- S03 can add the Decoder foundation using this lifecycle and the approved
  contract. Payload/container methods and their associated decoder-limit and
  argument sentinels remain for the dependent tasks. `AGENTS.md`, `STYLEGUIDE.md`,
  the Go version, and dependencies are unchanged.

## S03 — Decoder foundation and scalar reads

- [x] Complete S03.

**Read:** S01 contract, S02 handoff, scalar implementations/tests,
`errors.go`, and `read_fuzz_test.go`. **Deliverables:** `decoder.go` and scalar,
fragmentation, error-state, and lifecycle tests.

**Work:** implement construction, exact incremental reads, bounded scratch,
scalar decoding/conversion, and the chosen terminal-error/reset policy. Retain
wire classification precedence without promising to undo consumed bytes. Use
small literal fixtures and existing slice readers as complementary references.

**Acceptance:** test one-byte readers, every scalar truncation offset, EOF before
and after values, data plus EOF/error, temporary and repeated `(0, nil)` results,
and injected errors with `errors.Is`. Decode concatenated mixed scalars without
requesting bytes from the following value. Prove mismatch/overflow/error follow-up
behavior, stable zero-valued results on failure, and reset isolation if supported.

**Handoff (2026-11-01):**

- Added `decoder.go`, `decoder_limits.go`, `decoder_test.go`,
  `decoder_lifecycle_test.go`, and `decoder_test_reader_test.go`; extended
  `errors.go` with documented `ErrInvalidArgument`; updated this task. Production
  work uses the S01–S11 authority recorded in the approved contract.
- Public API added: `NewDecoder(r io.Reader) (d *Decoder)`, `Decoder`,
  `(*Decoder).Reset(r io.Reader)`, `SetLimits(limits DecoderLimits) (err error)`,
  `DecodeNil() (err error)`, and `DecodeBool`, `DecodeInt64`, `DecodeUint64`,
  `DecodeFloat32`, and `DecodeFloat64`, each returning its typed `(value, err)`.
  `DecoderLimits` exposes `MaxPayloadBytes uint64`, `MaxSkipDepth uint32`,
  `MaxSkipValues uint64`, and `MaxSkipBytes uint64` with the contract's finite
  defaults, payload maximum, and depth ceiling. No payload/container/Skip methods
  were added early; their limits are configured now for the reset lifecycle.
- The decoder uses fixed nine-byte scratch, reads a tag before classifying its
  accepted family, and requests only the remaining scalar body. Complete scalar
  conversions reuse the slice readers. Classification and overflow sentinels are
  direct and terminal. Source errors are contextual and terminal, and every
  subsequent scalar call returns the identical error without I/O or nonzero
  results. Unattached calls are direct, nonterminal `ErrUninitialized` failures.
- Clean boundary EOF is wrapped `io.EOF`; incomplete scalars are wrapped
  `io.ErrUnexpectedEOF` without matching `ErrTruncated` or `io.EOF`. Complete
  bytes accompanied by EOF succeed with pending EOF; any non-EOF error fails the
  current operation even with complete bytes. One hundred consecutive empty
  reads become wrapped `io.ErrNoProgress`, resetting the counter on progress.
  Reset clears failure/EOF, preserves limits, and never reads, closes, or drains.
  Valid/invalid `SetLimits` is atomic and cannot clear a terminal failure.
- Literal tests cover integer width boundaries/extremes, accepted wider signed
  and unsigned encodings, conversion overflow, float bits and widening, every
  accepted scalar form's truncation offsets, one-byte fragments, data/error/EOF
  combinations, transport-versus-classification precedence, temporary and
  terminal empty reads, precise mixed-scalar requests, sticky failures,
  zero/nil/typed-nil initialization, reset isolation, and stream ownership.
  Existing slice tests remain unchanged; no contract changes were required.
- Validation passed: `gofmt -w decoder.go decoder_limits.go decoder_test.go
  decoder_test_reader_test.go decoder_lifecycle_test.go errors.go`;
  `GOCACHE=/private/tmp/messagepack-go-cache go test ./...`;
  `GOCACHE=/private/tmp/messagepack-go-cache go vet ./...`;
  `git diff --check`; and `git diff --cached --check`. Reviewed the staged changes.
- S04 can proceed with encoder payloads. For S05, the private decoder helper
  `readFull(operation, dst, started) (n int, err error)` preserves partial counts
  and contextual errors without latching them; the current operation finalizes
  through `fail(err)`. This permits forwarding source bytes before selecting a
  simultaneous destination error. Header/body callers must use `started = true`
  after the tag, while new root tags use false. Payload/Skip work and performance
  evidence remain for later tasks. Policy, style, Go version, and dependencies
  are unchanged.

## S04 — Encode strings and binary without payload staging

- [x] Complete S04.

**Read:** S01 payload contract, S02 handoff, `string.go`, `bytes.go`, and their
boundary tests. **Deliverables:** payload methods in `encoder.go` plus tests.

**Work:** add in-memory string/byte writes and known-length reader-to-writer
payload transfers. Emit the appropriate header, then transfer in bounded chunks.
Avoid a string-to-byte conversion proportional to a large string's full size.
Validate wire lengths before output. Preserve invalid UTF-8, NULs, and the
empty-binary versus nil distinction.

**Acceptance:** check string thresholds 31/32, 255/256, 65535/65536 and binary
thresholds 255/256, 65535/65536. Validate oversized lengths without allocating a
huge object. Test empty, exact, short, and longer-than-declared sources; the extra
byte must remain unread. Inject failures in headers and mid-payload at both ends,
including partial progress counts. A generated large source and counting writer
prove bounded read requests and output before the entire source is consumed.

**Handoff (2026-11-01):**

- Extended `encoder.go` with the four payload methods and private bounded-copy
  helpers; broadened `ErrOverflow` GoDoc in `errors.go` to include streaming
  payloads without changing the sentinel. Added `encoder_payload_test.go`,
  `encoder_payload_writer_test.go`, `encoder_generated_reader_test.go`, and
  `encoder_counting_writer_test.go`; updated this task. Production work uses
  the S01–S11 authority recorded in the approved contract.
- Public API added: `(*Encoder).EncodeString(value string) (err error)`,
  `EncodeBytes(value []byte) (err error)`,
  `EncodeStringFrom(length uint64, src io.Reader) (n uint64, err error)`, and
  `EncodeBytesFrom(length uint64, src io.Reader) (n uint64, err error)`.
  Methods and helpers remain in `encoder.go`; no container methods were added.
- Headers reuse the existing slice header helpers and fixed nine-byte scratch.
  Binary slices are written in chunks no larger than 32768 bytes. String and
  reader transfers use a lazily allocated reusable 32768-byte copy buffer;
  no complete string-to-byte conversion, whole-payload staging, hidden output
  buffering, fast-path I/O delegation, flush, or close occurs. Reset may reuse
  this bounded scratch and retains no input reference.
- The stored terminal failure and attachment checks precede argument validation.
  Lengths above uint32 return direct nonterminal `ErrOverflow` before I/O,
  followed by a positive-length nil-source check. Zero length permits nil source
  and never reads it. Transfers consume exactly the declared payload and leave
  extra source bytes unread. Early EOF is wrapped `io.ErrUnexpectedEOF`; complete
  bytes plus EOF succeed, and source EOF is not retained between transfer calls.
- Progress counts only destination-accepted payload bytes, including partial or
  full writes accompanied by errors. Source-error bytes are offered to the
  destination first; a simultaneous destination error takes precedence. Source
  and destination failures are contextual and terminal, with no further I/O or
  drain. One hundred consecutive empty reads yield wrapped `io.ErrNoProgress`;
  short nil-error writes remain wrapped `io.ErrShortWrite`. No contract changed.
- Literal fixtures cover every string/binary header transition, raw invalid UTF-8
  and NUL bytes, nil versus empty binary, and consecutive payload/scalar values.
  Tests cover zero/exact/short/extra sources, huge declared lengths without huge
  allocations, argument/sticky/reset/typed-nil behavior, header and first/later
  payload-chunk faults at both ends, simultaneous error precedence, partial counts,
  zero progress, bounded requests, and caller ownership. An 8 MiB + 7-byte
  generated payload is validated incrementally without a payload-sized fixture;
  the first payload output occurs before the source is fully consumed.
- Validation passed: `gofmt -w encoder.go errors.go encoder_payload_test.go
  encoder_payload_writer_test.go encoder_generated_reader_test.go
  encoder_counting_writer_test.go`;
  `GOCACHE=/private/tmp/messagepack-go-cache go test ./...`;
  `GOCACHE=/private/tmp/messagepack-go-cache go vet ./...`;
  `git diff --check`; and `git diff --cached --check`. Reviewed the staged changes.
- S05 can add owned and writer-transfer decoding under the approved limits and
  use S03's count-preserving `readFull` helper before selecting the terminal
  source/destination error. `payloadBufferSize` is the shared private 32768-byte
  bound. Allocation/retained-memory measurements remain S10 work; these tests
  establish bounded requests and incremental output without a throughput or
  blanket zero-allocation claim. Policies, Go version, dependencies, and all
  slice implementations remain unchanged.

## S05 — Decode owned payloads and stream payloads to writers

- [x] Complete S05.

**Read:** S01 payload/limits/error contract, S03–S04 handoffs, `string.go`,
`bytes.go`, and their tests. **Deliverables:** payload methods in `decoder.go`
and ownership, size-limit, and transfer tests.

**Work:** parse lengths incrementally. Add convenience reads with owned storage
and configured allocation limits, plus transfers to a supplied writer using
bounded memory. Validate lengths and representability before allocating. Keep
payload caps for materializing results separate from streaming transfer controls.

**Acceptance:** cover all payload header forms, empty values, invalid UTF-8,
header/payload truncation, advertised huge lengths backed by tiny input, and
configured-limit boundaries. Owned results survive later decodes/reset. Streaming
an accepted payload leaves the next encoded value untouched; destination failure
reports correct progress and terminal state without draining the remaining input.
Exercise source and destination faults, zero-length transfers, and small fragments.
Demonstrate a large transfer without allocating a payload-sized result or fixture.

**Handoff (2026-11-01):**

- Extended `decoder.go` and `errors.go`; added `decoder_payload_test.go`,
  `decoder_payload_io_test.go`, and `decoder_counting_writer_test.go`; updated
  this task. Production work uses the authority recorded in the S01 contract.
- Public API added: `(*Decoder).DecodeString() (value string, err error)`,
  `DecodeBytes() (value []byte, err error)`, and `DecodeStringTo(dst io.Writer)`
  and `DecodeBytesTo(dst io.Writer)`, both returning `(n uint64, err error)`.
  Added documented direct sentinel `ErrLimitExceeded` without changing existing
  sentinels, slice APIs, or the approved contract.
- Payload headers are parsed incrementally without consuming following values.
  Owned results check `MaxPayloadBytes` before platform `int` representability
  and allocation, return zero values on failure, and remain stable across source
  mutation, later decodes, scratch reuse, and reset. Empty binary returns a non-nil
  empty slice. Owned reads use requests of at most 32768 bytes; owned strings use
  safe conversion and intentionally may allocate proportionally to payload size.
- Writer transfers ignore materialization/Skip limits and lazily reuse at most
  32768 bytes of payload scratch. A required nil destination is rejected before
  reading any header, even for empty values, after existing failure/attachment
  checks. Empty transfers do not write or allocate payload scratch. Counts exclude
  headers and include bytes accepted with destination errors. Source-error bytes
  are forwarded first; simultaneous destination errors take precedence. Failures
  are contextual and sticky, without retries, extra value reads, or draining.
- Literal tests cover all payload header forms, thresholds, wider encodings,
  empty values, invalid UTF-8/NULs, and every truncation offset of small fixtures.
  Tests cover ownership, cap boundaries, huge advertised lengths with tiny input,
  reserved/extensions/mismatches, nil/typed-nil destinations, pending EOF,
  source/destination faults, partial/full/short writes, simultaneous errors,
  first/later chunk progress, and the 99/100 empty-read boundary. Generated
  8 MiB + 7-byte transfers validate every byte incrementally with a one-byte
  materialization cap, bounded requests, output before full source consumption,
  and an intact following value. Failed generated transfers stop after the
  failed chunk without draining the remaining source.
- Validation passed: `gofmt -w decoder.go errors.go decoder_counting_writer_test.go
  decoder_payload_test.go decoder_payload_io_test.go`;
  `GOCACHE=/private/tmp/messagepack-go-cache go test ./...`;
  `GOCACHE=/private/tmp/messagepack-go-cache go vet ./...`;
  `GOCACHE=/private/tmp/messagepack-go-cache go test -run
  'TestDecoderOwnedPayloadIntOverflow|TestDecoderGeneratedPayloadTransfer|TestDecoderPayload'
  -v ./...`; `git diff --check`; and `git diff --cached --check`.
  The explicit owned-length/int-overflow test skips on this 64-bit platform,
  where all uint32 lengths fit `int`; 32-bit execution remains S09 work.
- S06 can add container headers and streaming-schema tests using these typed
  payload operations. Buffer/allocation measurements remain S10 work; current
  tests establish bounded requests and incremental behavior without a blanket
  allocation claim. Policies, Go version, dependencies, and slice implementations
  remain unchanged.

## S06 — Stream container contents and complete typed messages

- [ ] Complete S06.

**Read:** S01 contract, S02–S05 handoffs, `array.go`, `map.go`,
`application_message_test.go`, and `integration_test.go`. **Deliverables:** header
methods on Encoder/Decoder and new explicit streaming-schema integration tests.

**Work:** add array/map header methods and encode/decode the existing six-field
application fixture through typed stream calls. Process children as they arrive;
do not create a generic object tree. Keep the original slice-based fixture/tests.
Add a generated large array whose elements can be processed one at a time.

**Acceptance:** exact bytes match the existing fixture, and stream/slice decoding
interoperate both ways. Test empty containers, 15/16 and 65535/65536 boundaries,
uint32 maximum headers without constructing their contents, nested containers,
and multiple complete messages on one stream. Missing children must fail the
schema decoder even if the header succeeds. Show missing map values, wrong counts,
and truncation between fields. Demonstrate processing before the entire container
is available, without collecting all elements. Check map arithmetic on 32-bit.

## S07 — Skip unknown supported values with bounded work

- [ ] Complete S07.

**Read:** S01 skip limits, S03/S05/S06 parsing and handoffs, `errors.go`, and
current application-schema tests. **Deliverables:** `Decoder.Skip` per the
contract, nested skip tests, and a forward-compatible schema example/test.

**Work:** consume exactly one supported value, including nested containers and
payloads, without materializing it. Enforce the contracted depth and work/byte
limits with checked counters. Use bounded traversal state; recursion, if chosen,
must be depth-checked before descending. Skip map keys and values, not just pairs
as single child values. Keep extension rejection consistent with typed decoding.

**Acceptance:** cover every supported family, all payload/container header widths,
empty/nested values, truncated children, maximum declared counts, limits at and
above boundaries, reserved/extension tags, and I/O errors. A sentinel value after
the skipped value must decode intact. The new schema example accepts reordered
fields and skips unknown fields while explicitly rejecting duplicates and missing
required fields. Preserve the old strict-schema tests as examples of that policy.

## S08 — Prove HTTP, file, and backpressure workflows

- [ ] Complete S08.

**Read:** streaming contract and S02–S07 handoffs, `example_test.go`,
`integration_test.go`, and the application fixture. **Deliverables:** executable
streaming examples and integration tests, with helpers in appropriate test files.

**Work:**

- Use a local `httptest` server/client to decode directly from `resp.Body`, including
  an incrementally delivered response and a large binary value copied to a sink.
  The caller closes the body and controls cancellation/deadlines.
- Write a large generated array/object and a known-length binary payload to a file
  under `t.TempDir`, using caller-owned buffering where useful. Check encode,
  caller flush, and close errors; reopen and process incrementally for validation.
- Use `io.Pipe` or an equivalent deterministic gated stream to prove a consumer
  receives fields/chunks before the producer finishes. Use deadlines/cleanup to
  prevent hangs and release every goroutine on failures; avoid timing-only sleeps.
- Show both a sequence of application messages and an application requiring exactly
  one message. Trailing-data checks need a finite body or external framing; do not
  wait for EOF after each value on a persistent stream.

**Acceptance:** examples compile and produce verified output; network tests remain
local; large data is generated/checked incrementally without full-size buffers.
No body-wide `ReadAll` or whole-object encode buffer is used in the stream examples.
Test truncated responses, failing destinations, cancellation, caller ownership,
and continued use of supplied buffering. Run examples and `go test -race ./...`.

## S09 — Fuzz stream boundaries and verify portability

- [ ] Complete S09.

**Read:** S01 error/state rules, new stream tests, `read_fuzz_test.go`,
`roundtrip_fuzz_test.go`, and `docs/poc-fuzzing.md`. **Deliverables:** stream fuzz
targets and `docs/streaming-fuzzing.md` with reproducible results.

**Work:** fuzz typed decoding, transfer/skip, valid typed round trips, and fragmented
I/O schedules. Seed from literal boundaries, malformed tags, nested containers,
truncated lengths, and consecutive values. Bound input size, depth, work, and
materialized output so fuzzing cannot allocate from hostile lengths or hang.
Compare successful values/wire bytes with the existing slice API where semantics
match; compare errors only where the contracts intentionally match.

**Acceptance:** invariants cover no panic/unbounded allocation, correct value
boundaries, error identity, terminal-state behavior, ownership, and bounded work.
Run each new fuzz target separately for at least 30 seconds; record commands,
durations, toolchain, platform, and failures. Run tests on a supported 32-bit target
when available; otherwise record cross-compilation separately from execution.
Preserve reproducing inputs and resolve discovered defects under the applicable
authority before completion. Existing fuzz seed tests must still pass.

## S10 — Measure streaming memory and throughput

- [ ] Complete S10.

**Read:** streaming implementation, `benchmark_test.go`, `allocation_test.go`,
`docs/poc-benchmarks.md`, and S08/S09 handoffs. **Deliverables:** stream benchmarks,
justified memory regression tests, and `docs/streaming-benchmarks.md`.

**Work:** measure primitive sequences, small messages, large containers, owned
payload reads, and reader-to-writer payload transfers. Compare caller-buffered
and unbuffered I/O. Separate construction from reuse/reset where supported.
Include at least three substantially different payload sizes using generated
sources and counting/discard sinks; avoid measuring a giant fixture buffer.

**Acceptance:** report throughput, B/op, allocs/op, and read/write call counts
with workload and environment. Use at least five benchmark samples. For transfer
paths, show retained working memory and allocation bytes do not grow with payload
length; allocation count alone is insufficient evidence. Explain any profiles or
measurement limits, and keep owned-result allocation proportionality explicit.
Retain the existing slice allocation guarantees. Do not claim blanket zero
allocations or add speculative pooling/unsafe optimizations to meet a target.

## S11 — Publish streaming-first documentation and final review

- [ ] Complete S11.

**Read:** every handoff, `docs/streaming-contract.md`, new examples/reports,
`README.md`, `doc.go`, and the archived PoC contract. **Deliverables:** updated
README/package GoDoc and a completed plan with verification evidence.

**Work:** lead with Encoder/Decoder use on response bodies and files, then explain
small typed operations, container traversal, large payload transfers, and skipping.
Describe exact lengths/counts, buffering and ownership, limits, partial failures,
EOF versus incomplete objects, terminal-state/reset rules, and how a caller checks
complete-message consumption. Present the slice API as an available lower-level
option with its distinct ownership/error behavior. Keep historical PoC reports
clearly historical and fix claims that streaming is still deferred.

**Acceptance:** examples use actual implemented APIs; documentation matches tests;
all S01–S10 handoffs exist; supported and deferred formats/features are explicit.
Run `go build ./...`, `go test ./...`, `go vet ./...`, executable examples,
`go test -race ./...`, formatting and link checks, and `git diff --check`. Confirm
both requested use cases work with bounded codec memory and without regressing the
slice API. Record remaining platform/measurement limitations without marking
unverified claims as proven. Preserve `AGENTS.md` and `STYLEGUIDE.md` unchanged.
