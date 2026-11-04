# Streaming MessagePack contract

## Status and authority

**Approved by the maintainer on 2026-11-01. S01–S11 are complete.**
The API, lifecycle, limits, and implementation constraints below are the approved
S01 contract for [streaming tasks S01–S11](../TODO.md).

The initial review draft was committed as `8044d3d` without production changes.
The assistant then asked: “Do you approve this contract and explicitly authorize
agents to modify production code for S01–S11?” The maintainer replied:

> Please continue

That reply approves the reviewed contract and grants the scoped production-work
authorization requested in the question. Agents may make the production changes,
tests, and documentation needed to complete S01–S11 under this contract, using
one fresh agent per task and committing the changes as requested. This permission
is limited to those streaming tasks; it does not authorize unrelated production
changes or expand the deferred scope below.

The standing [agent policy](../AGENTS.md) and [style guide](../STYLEGUIDE.md) remain
unchanged. This session-specific authorization overrides the production-code
restriction only for S01–S11. The earlier authority in the [PoC contract](poc-contract.md)
still covers only its archived tasks 1–11 and is not the basis for streaming work.
S01–S11 are complete with implementation, examples, tests, measurement evidence,
user documentation, and final review in their [handoffs](../TODO.md). Approval
itself was separate from that implementation evidence.

The module remains `github.com/itsmontoya/messagepack`, package `messagepack`,
using the existing Go 1.27.2 baseline and standard library only. Toolchain,
dependency, policy, and style-guide changes remain outside this authorization.

## Scope and compatibility

Add typed streaming operations for nil, bool, signed/unsigned 64-bit integers,
float32/float64, strings, binary, and container headers. Add bounded copying and
skipping of one complete supported value. Callers retain responsibility for their
schemas and for iterating container children. There is no generic object tree,
reflection, `Encode(any)`/`Decode(any)`, automatic mapping, struct tags, extension
support, pooling, code generation, or shared-codec concurrency in this phase.

Keep every existing exported slice function, sentinel, wire choice, conversion,
ownership guarantee, and test compatible with the PoC contract. Scalar/header
helpers may be reused with fixed scratch storage. Streaming a payload or complete
object must not first accumulate it for a slice operation. Stream EOF and failure
consumption have their own rules below; slice failures still consume zero bytes.

The streaming APIs do not validate container completeness, required fields,
duplicates, ordering, or trailing data. A schema decoder must count and validate
children explicitly. A complete container header can succeed with no children
present, as it does in the slice API.

## Public surface and file placement

`Encoder` and `Decoder` are concrete exported types with unexported state.
Constructors sit immediately above their type declarations. All methods belonging
to `Encoder` live in `encoder.go`; all `Decoder` methods live in `decoder.go`,
including private helpers. Exported symbols receive concise GoDoc. New
sentinels live with the existing sentinels in `errors.go`.

```go
func NewEncoder(w io.Writer) (e *Encoder)
func (e *Encoder) Reset(w io.Writer)

func (e *Encoder) EncodeNil() (err error)
func (e *Encoder) EncodeBool(value bool) (err error)
func (e *Encoder) EncodeInt64(value int64) (err error)
func (e *Encoder) EncodeUint64(value uint64) (err error)
func (e *Encoder) EncodeFloat32(value float32) (err error)
func (e *Encoder) EncodeFloat64(value float64) (err error)
func (e *Encoder) EncodeString(value string) (err error)
func (e *Encoder) EncodeBytes(value []byte) (err error)
func (e *Encoder) EncodeStringFrom(length uint64, src io.Reader) (n uint64, err error)
func (e *Encoder) EncodeBytesFrom(length uint64, src io.Reader) (n uint64, err error)
func (e *Encoder) EncodeArrayHeader(count uint32) (err error)
func (e *Encoder) EncodeMapHeader(count uint32) (err error)

func NewDecoder(r io.Reader) (d *Decoder)
func (d *Decoder) Reset(r io.Reader)
func (d *Decoder) SetLimits(limits DecoderLimits) (err error)

func (d *Decoder) DecodeNil() (err error)
func (d *Decoder) DecodeBool() (value bool, err error)
func (d *Decoder) DecodeInt64() (value int64, err error)
func (d *Decoder) DecodeUint64() (value uint64, err error)
func (d *Decoder) DecodeFloat32() (value float32, err error)
func (d *Decoder) DecodeFloat64() (value float64, err error)
func (d *Decoder) DecodeString() (value string, err error)
func (d *Decoder) DecodeBytes() (value []byte, err error)
func (d *Decoder) DecodeStringTo(dst io.Writer) (n uint64, err error)
func (d *Decoder) DecodeBytesTo(dst io.Writer) (n uint64, err error)
func (d *Decoder) DecodeArrayHeader() (count uint32, err error)
func (d *Decoder) DecodeMapHeader() (count uint32, err error)
func (d *Decoder) Skip() (err error)
```

`DecoderLimits` has its own `decoder_limits.go` file. Its entire public
shape is:

```go
type DecoderLimits struct {
    MaxPayloadBytes uint64
    MaxSkipDepth    uint32
    MaxSkipValues   uint64
    MaxSkipBytes    uint64
}
```

No configuration framework, exported option functions, error accessor, flush,
close, or whole-object method is included. Each operation reports its own error.
`Reset` and `SetLimits` are the only lifecycle/configuration operations.

## Initialization, ownership, and reset

- A zero-value codec is unattached. A constructor attaches its argument and starts
  with no terminal failure. `NewDecoder` uses the defaults below. Constructors
  return usable codec pointers even when given a nil interface; such instances
  remain unattached until reset with a non-nil stream.
- Encoding/decoding/skipping on an unattached instance returns
  `ErrUninitialized` directly, performs no I/O, and does not latch a failure.
  `SetLimits` is allowed on an unattached Decoder.
- `Reset` clears the terminal error, pending EOF, per-operation progress, and
  traversal state, and attaches the new stream. A nil interface leaves the codec
  unattached. Decoder limits survive reset; resetting a never-configured zero
  value initializes default limits. Scratch storage may be reused.
- Reset does not read, write, flush, close, rewind, drain, or repair either stream.
  Resetting onto a partially consumed/written message does not restore alignment;
  callers must supply a known value boundary themselves.
- Literal nil interfaces are recognized without reflection. Interfaces containing
  typed nil pointers are treated as non-nil: their methods retain ordinary Go
  behavior, including any panic. Callers must supply valid reader/writer values.
  Nil codec pointer receivers are outside the contract; zero-value instances are
  the supported unattached form.
- The caller owns every supplied reader/writer and its lifetime. The codec never
  closes or flushes them, launches background goroutines, or supplies concurrent
  safety. Codec instances must not be copied after first use or used concurrently.
- `DecodeString` and `DecodeBytes` return owned results stable across subsequent
  calls and reset. Failed materializing reads return `""` or nil. Successful empty
  binary returns a non-nil empty slice. There are no borrowed stream results.
  In-memory encoding does not retain input after the method returns.

## Limits and bounded memory

Each zero field in a `DecoderLimits` argument selects that field's default.
Zero never means unlimited. `SetLimits` validates and replaces the complete
configuration atomically, without I/O; it is allowed before initialization and
after terminal failure. It cannot clear a terminal failure. Invalid configuration
returns `ErrInvalidArgument` directly and leaves all previous limits and
stream state unchanged. There is no partial update.

| Field | Default | Valid effective values | Applies to |
| --- | --- | --- | --- |
| `MaxPayloadBytes` | 16 MiB (`16777216`) | 1–`4294967295` bytes | Each `DecodeString`/`DecodeBytes` result only. |
| `MaxSkipDepth` | 64 | 1–256 container levels | One `Skip` call; 256 is a fixed hard ceiling. |
| `MaxSkipValues` | `1048576` | 1–`18446744073709551615` values | One `Skip` call, including its root and every key/value separately. |
| `MaxSkipBytes` | 64 MiB (`67108864`) | 1–`18446744073709551615` bytes | All wire bytes consumed by one `Skip`, including tags and headers. |

The payload materialization cap is checked immediately after parsing an accepted
length, before payload allocation or reads. If over cap, return `ErrLimitExceeded`.
Next check `int` representability; an accepted length exceeding platform `int`
returns `ErrOverflow`. Both errors are terminal because the header was consumed.
A complete hostile header backed by tiny input therefore cannot trigger an
allocation above the configured cap. Configuring a large cap is an explicit
caller memory policy, not a promise that arbitrary large allocations will succeed.

Transfer APIs do not use `MaxPayloadBytes` or the Skip limits. They accept any
valid uint32 wire payload length, including above 32-bit `int`, through bounded
chunks. Codec payload-copy buffers and maximum payload read/write requests are
32 KiB (`32768`) each. Scalars use at most nine wire bytes and headers at most
five; no buffer grows with total output, container count, or payload length.
Skip traversal uses bounded state with at most 256 container frames. Convenience
owned results intentionally allocate proportionally to their accepted lengths.

No transfer path uses `io.ReadAll`, a whole-payload string-to-byte conversion,
whole-object staging, or unbounded `WriterTo`/`ReaderFrom` delegation. Large strings
are written in bounded chunks even if their in-memory source is already complete.
Scratch allocation/reuse strategy is private. S10's
[measurement report](streaming-benchmarks.md) records actual allocation bytes
and retained working memory across three payload sizes, with scoped results and
measurement limits rather than a blanket allocation promise.

No typed container-header operation allocates children or applies Skip limits.
Map child calculations use `uint64(count) * 2`, never `int` or uint32 multiplication.
Counters use checked wide arithmetic, and chunk sizes are bounded before narrowing
to `int` on every platform.

Callers bound complete request/response sizes through their own reader wrappers
and framing. They apply transport deadlines, cancellation, or context-aware I/O
at that boundary. Limits cannot interrupt an underlying blocking `Read`/`Write`.

## Payload transfer and progress

All lengths are byte lengths. `EncodeStringFrom` and `EncodeBytesFrom` validate
`length <= 4294967295` before emitting a header or reading the source. An oversized
length returns `ErrOverflow` without changing state. For positive lengths a nil
source interface returns `ErrInvalidArgument` before I/O. Length validation takes
precedence over that nil-source check. For zero length the nil source is allowed:
write the empty-family header, do not call the source, and return `n == 0`.
`EncodeString` and `EncodeBytes` apply the same wire-length validation before any
output, retaining the existing slice encoder's rejection of oversized payloads.

For a positive declared length, emit the smallest family header, then read and
write exactly that many payload bytes on success. Never probe beyond the declared
length, even to check EOF. Extra source bytes remain unread. Early source EOF is
`io.ErrUnexpectedEOF`, including when no payload byte was supplied after the
header. Source failure after a header is terminal; the escaped prefix stays written.
Source bytes accompanied by `io.EOF` complete successfully if they supply exactly
the remaining declared payload. Encoder source EOF is not retained across separate
transfer calls, each of which receives its own source argument.

`DecodeStringTo` and `DecodeBytesTo` require a non-nil destination interface before
reading any tag, even for an eventually empty payload. A nil destination returns
`ErrInvalidArgument` without consuming input or latching an error. An accepted
header is followed by exactly its declared payload, with no read of a following
value. Zero-length wire payloads require no destination `Write` and succeed with
`n == 0`. `EncodeBytes(nil)` means empty binary; only `EncodeNil` means nil.

For all four transfer methods, `n` counts **payload bytes accepted by the
destination writer**, excluding every header byte. On success it equals the
declared wire length. On header failure it is zero. Count accepted bytes even
when their `Write` also returns an error. Ordinary scalar/owned/header decode
results are zero-valued on any failure; transfer progress is the exception.

The source may already have supplied more payload bytes than the destination
accepted on a failed chunk. The gap is at most one 32 KiB chunk. The returned
count does not promise source-position rollback or resumability. On destination
failure, perform no further source reads or destination writes and do not drain
the remainder. No retry of a whole value is supported after a prefix escapes.

For a read that returns bytes and an error, account for and offer those bytes to
the transfer destination before reporting the source error. If the destination
also fails while accepting them, return the destination error, preserving its
`errors.Is` identity; destination failure takes precedence over that simultaneous
source error. This precedence is deliberate and does not promise both errors in
one result. A complete transfer may have `n == length` and still fail because of
a simultaneous non-EOF source or destination error.

## Incremental I/O and error observation

There is no hidden input read-ahead or buffered output. Each read request is at
most the bytes remaining in the current tag, header, scalar body, or payload
chunk. Caller-supplied `bufio.Reader`/`bufio.Writer` may read ahead or retain output;
the caller must keep using those wrappers for subsequent access and perform its
own flush. Encode success means the writer accepted bytes, not durable storage.

Fragmented reads are retried until the required segment completes or fails.
After 100 consecutive `(0, nil)` reads without any intervening progress, return
context-wrapped `io.ErrNoProgress` and latch it. Progress resets that counter.
No repeated zero-progress write retry exists: every `Write` returning fewer
bytes than requested with nil error immediately becomes `io.ErrShortWrite`.
Partial/full writes with non-nil errors always fail; accepted bytes still count.
Reader/writer implementations must obey the standard `io.Reader`/`io.Writer`
count contracts; behavior for invalid negative or oversized counts is outside
this contract.

| Underlying read result | Observation by the current operation |
| --- | --- |
| Bytes with nil error, segment complete | Continue parsing or complete the operation. |
| Bytes with `io.EOF`, enough for the entire requested value | Complete successfully; Decoder retains pending EOF for the next operation without an extra underlying read. Encoder transfers retain no source EOF across calls. |
| Bytes with `io.EOF`, enough for a segment but not the entire requested value | Use the segment; the next required read observes EOF and the operation fails as incomplete. |
| `io.EOF` before any byte of a new typed operation/Skip root | Return context-wrapped `io.EOF`; no `ErrTruncated` match. |
| `io.EOF` after the current operation began, including a missing Skip child | Return context-wrapped `io.ErrUnexpectedEOF`; no `io.EOF` or `ErrTruncated` match. |
| Bytes with a non-EOF error, even enough for the entire operation | Fail the current operation with that context-wrapped error. Transfer methods first offer those bytes to their destination as specified above. |
| Non-EOF error without bytes | Fail immediately with the context-wrapped error. |

For scalar/header/owned operations, a non-EOF transport error takes precedence
over classification or conversion of the bytes from that failing read. No pending
non-EOF transport error is silently discarded or deferred to another operation.
Any bytes already consumed remain consumed, even if the ordinary result is zero.
Only EOF accompanying all required bytes permits successful completion.

Typed container calls do not remember an application message's start: EOF before
a required child call is still `io.EOF` at the codec boundary. The schema decoder
must convert that condition into its own incomplete-message error. `Skip` does
remember its root's completeness, so missing children there are unexpected EOF.

## Error identity, precedence, and state transitions

Keep the five existing slice sentinels unchanged. Add three additional direct
sentinels, each with GoDoc: `ErrUninitialized` for no attached stream,
`ErrInvalidArgument` for invalid limits or required nil transfer interfaces, and
`ErrLimitExceeded` for consumed values exceeding configured decoder limits.
Error strings are not part of the contract.

Wrap external source/destination errors with operation and side context using
`%w`, preserving `errors.Is`. This includes EOF, unexpected EOF, short writes,
and no progress. Return codec/state sentinels directly. In particular, stream
unexpected EOF does **not** also match the existing slice-only `ErrTruncated`.

Value operations first return an existing terminal failure, then check attachment,
then validate pre-I/O arguments in the order described above. The following table
defines whether subsequent value operations can proceed:

| Event | Result/state | Later value operations |
| --- | --- | --- |
| New/reset with non-nil stream | Ready, no I/O | May perform I/O. |
| Unattached operation | `ErrUninitialized`, unattached | No I/O until reset attaches a stream. |
| Pre-I/O invalid argument/encode length | Direct sentinel, state unchanged | May proceed with corrected input. |
| Valid operation | Success, ready; possibly pending EOF | May proceed; pending EOF is observed without another read. |
| Clean EOF before a new value | Wrapped `io.EOF`, terminal | Return the same stored error without I/O. |
| Decode classification, conversion, or consumed length/Skip limit failure | Direct sentinel, terminal | Return the same stored error without I/O. |
| Any read/write failure, including zero-byte failure or source failure after an encode header | Context-wrapped I/O error, terminal | Return the same stored error without I/O. |
| `SetLimits`, valid or invalid | Update limits atomically or return `ErrInvalidArgument`; I/O state unchanged | Existing terminal failure, if any, persists. |
| `Reset` | Clear failure and pending EOF; preserve limits; attach argument | May proceed if attached. |

There is no rollback, retry exception for EOF, automatic resynchronization, or
terminal-error escape through argument validation. Pre-I/O validation is the only
value-operation failure that leaves an attached healthy codec ready. Even a
zero-byte transport error is terminal because transport recovery is caller-owned.

After a tag was obtained without a non-EOF transport error, classification order
matches the slice API: reserved `0xc1`, then extension tags, then supported-family
mismatch. A mismatch is terminal after consuming only the tag; it does not parse
the wrong family's length or body. Expected-family truncation precedes numeric
overflow. Limits are checked only once the necessary accepted header exists.

| Condition | Result | Possible consumption and effect |
| --- | --- | --- |
| No next tag byte | Wrapped `io.EOF` | No byte consumed by this operation; terminal. |
| Expected header/scalar/payload incomplete | Wrapped `io.ErrUnexpectedEOF` | Available prefix consumed; terminal. |
| Reserved tag | `ErrInvalidFormat` | Tag consumed; terminal. |
| Extension tag, including timestamps | `ErrUnsupportedType` | Tag consumed, no extension body read; terminal, including Skip. |
| Other supported family | `ErrTypeMismatch` | Tag consumed; terminal. Skip accepts every supported family. |
| Complete integer outside requested range | `ErrOverflow` | Complete scalar consumed; terminal. |
| Owned payload length above configured cap | `ErrLimitExceeded` | Complete accepted header consumed; no payload allocation/read; terminal. |
| Accepted owned length above platform `int` | `ErrOverflow` | Complete header consumed; no payload allocation/read; terminal. |
| Skip depth/value/byte ceiling | `ErrLimitExceeded` | At most the permitted prefix consumed; terminal, no draining. |
| Encode length above wire limit | `ErrOverflow` | No source read/output, not terminal. |
| Transfer source non-EOF I/O failure | Context-wrapped source error | Prefix may have moved; terminal; transfer count retained. |
| Destination failure/short write | Context-wrapped destination error/`io.ErrShortWrite` | Accepted prefix remains; terminal; transfer count retained. |
| Repeated zero-progress source reads | Context-wrapped `io.ErrNoProgress` | No further I/O after threshold; terminal. |

## Skip accounting and value boundaries

`Skip` consumes one complete supported value and leaves the following encoded
value untouched. It skips integer bits without applying a signed/unsigned
conversion, consumes float bodies without coercion, streams string/binary bytes
without materializing them, and walks every array child and map key/value.
Reserved and extension tags retain the classification above; skipping does not
add extension support.

Budgets restart at each `Skip` call. Every value, including the root and every
container, counts once toward `MaxSkipValues`. Depth is the number of containing
array/map levels including the current container: a root scalar has depth zero,
a root array/map (including empty) has depth one, and a directly nested container
has depth two. An attempted container above the limit fails after at most its
header, before visiting children. Scalars under an allowed deepest container do
not create another container level.

All consumed wire bytes count toward `MaxSkipBytes`. Check the remaining budget
before requesting any segment, so the codec never reads a byte above the cap.
After an accepted payload length is known, reject a payload that cannot fit the
remaining byte budget before reading it. Before traversing container children,
use checked uint64 arithmetic to include their declared count (twice the pair
count for maps) with all outstanding ancestor siblings. Reject if visited plus
outstanding values exceed the value budget, or if the minimum one tag byte per
outstanding value exceeds the remaining byte budget. These are lower bounds;
larger child bodies are checked as their tags/headers reveal them.

No advertised count allocates an array/map or a count-sized traversal stack. The
hard depth cap bounds traversal memory even if a caller raises other budgets.
A declared count/length can therefore fail its budget after its header, without
iterating through a huge count or requiring the promised bytes to exist. If a
budget prevents learning the rest of a header, the budget error wins over a
truncation or limit condition that would require reading additional bytes.

## Wire reference and conversion compatibility

Checked 2026-11-01 against the [official MessagePack specification](https://github.com/msgpack/msgpack/blob/master/spec.md),
including its format overview, integer/float layouts, payload/container families,
limits, and type-to-format rules. The existing tag assignments, big-endian fields,
smallest integer/header selection, uint32 payload/count ceiling, and map pair
interpretation agree with that reference. No wire incompatibility was identified.
The specification permits implementation-defined handling of invalid string
bytes; preserving them and exposing `DecodeStringTo` retains their original bytes.
Extensions and timestamps are deliberately rejected as a supported-subset policy.

Retain every conversion in the PoC contract: nonnegative signed encodes equal
unsigned encodes; all fitting integer wire widths decode; negative-to-unsigned
and excessive unsigned-to-signed values fail. Float widths/bits are preserved;
float32 may widen to float64 with unspecified widened NaN payload bits. Float64
to float32 and integer/float coercions remain type mismatches. String and binary
remain distinct, byte-preserving families with no UTF-8 normalization.

## HTTP, files, and complete-message workflows

For an HTTP response, the caller constructs a Decoder directly on `resp.Body`,
sets an appropriate materialization cap for small fields, and traverses a known
schema using typed calls. It copies a large binary/string directly to a sink with
`DecodeBytesTo`/`DecodeStringTo`. The caller closes the body and supplies request
context/deadlines. S05 proves transfer bounds; S06 proves complete typed messages;
S08 proves local incremental HTTP, truncation, cancellation, and sink failures.

For files, the caller opens a file, optionally wraps it in `bufio.Writer`, and
encodes headers and children incrementally. A known-length payload streams from
a reader through `EncodeBytesFrom`/`EncodeStringFrom`. The caller checks encode,
flush, and close errors separately; durability requires caller-owned file policy.
Reopening with a Decoder permits incremental validation without a whole-file
buffer. S04 proves payload output bounds; S06 proves generated arrays; S08 proves
temporary-file buffering, ownership, and processing before producer completion.

Lengths/counts precede contents. Unknown-length sources require an application
chunk/sequence schema or external counting/staging. The codec does not spool,
seek back, invent indefinite-length encoding, or silently materialize them.
Datasets above one uint32 payload/count use multiple bounded values or containers.

A sequence consumer decodes complete schema messages consecutively; EOF before
the next root is normal termination, while EOF during required children is an
application error. A consumer requiring exactly one message checks for trailing
data only inside a finite body or external frame, using the same supplied buffered
reader if present. It must not wait for EOF after every value on a persistent
stream. `Skip` supports unknown fields; duplicate/required-field rules remain in
the caller's schema. S07 proves that schema policy and S08 proves both sequence
and exactly-one-message usage.

S09's [fuzz report](streaming-fuzzing.md) records four bounded smoke runs on
November 1–2, 2026 covering fragmented I/O, hostile lengths, boundaries, ownership,
and terminal state. Linux/386 tests cross-compiled; no compatible runtime was
available for execution. S10's [measurement report](streaming-benchmarks.md)
records five samples for each of 91 cases on November 2, 2026, including transfer
allocation bytes and retained memory at 1 KiB, 64 KiB, and 8 MiB, plus proportional
owned-result costs. These results apply to their stated workloads and environment.
The [README](../README.md) and package GoDoc publish the implemented streaming
workflow; final S11 verification is recorded in [the task handoff](../TODO.md).
