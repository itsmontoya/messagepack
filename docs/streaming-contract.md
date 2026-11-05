# Streaming MessagePack contract

## Status and authority

**S01–S11 were approved on 2026-11-01 and are complete. API revised on 2026-11-04.**
The original streaming tasks and their implementation evidence remain recorded in
their [handoffs](../TODO.md). The maintainer subsequently requested that the typed
Encoder become a concrete Writer, with Encoder exposing only Encode and Close.
Close leaves the supplied output open. The maintainer also requested moving all
Append encoding logic into Writer methods and removing the package-level Append functions.
The maintainer then requested the same decoding split: concrete Reader, Decoder
with Decode and Close, Decodee hooks, and removal of the package-level Read API.
The maintainer subsequently requested reflection as a fallback when no Encodee
or Decodee hook matches. Its mapping and limits are documented below and in the
[README](../README.md#reflect-ordinary-go-values).
The public surface and lifecycle below reflect those revisions.

The maintainer also requested updating the [agent policy](../AGENTS.md) to permit
production work. The [style guide](../STYLEGUIDE.md) still applies.

The module remains `github.com/itsmontoya/messagepack`, package `messagepack`,
using the existing Go 1.27.2 baseline and standard library only. Toolchain,
dependency, and style-guide changes are outside this refactor.

## Scope and compatibility

Add typed streaming operations for nil, bool, signed/unsigned 64-bit integers,
float32/float64, strings, binary, and container headers. Add bounded copying and
skipping of one complete supported value. Callers retain responsibility for their
schemas and for iterating container children when using the typed API. Encoder
and Decoder also provide reflection mapping. Struct tags, extension support,
pooling, code generation, and shared-codec concurrency remain outside scope.

Writer and Reader methods own encoding and decoding directly. The package-level
Append and Read functions and their slice-only helpers are removed. ReaderLimits
replaces DecoderLimits. Scalar and header operations use fixed scratch storage;
streaming a payload or complete object must not first accumulate it in memory.
Reader payload results own their storage, and stream failures may consume a prefix.
ErrTruncated is removed; clean and incomplete input use the EOF rules below.

The streaming APIs do not validate container completeness, required fields,
duplicates, ordering, or trailing data. A schema decoder must count and validate
children explicitly. A complete container header can succeed with no children
present.

## Public surface and file placement

`Encoder`, `Decoder`, `Writer`, and `Reader` are concrete exported types with unexported state.
Constructors sit immediately above their type declarations. All methods belonging
to `Writer` live in `writer.go`; all `Reader` methods live in `reader.go`,
including private helpers. Encoder methods live in `encoder.go`; Encodee lives
in `encodee.go`. Decoder methods live in `decoder.go`, and Decodee lives in
`decodee.go`. Shared private state lives in `writer_state.go` and `reader_state.go`.
Exported symbols receive concise GoDoc. New sentinels live with the existing
sentinels in `errors.go`.

```go
func NewEncoder(w io.Writer) (out *Encoder)
func (e *Encoder) Encode(value any) (err error)
func (e *Encoder) Close() (err error)

type Encodee interface {
    EncodeMessagePack(Writer) error
}

func NewWriter(dst io.Writer) (out *Writer)
func (w *Writer) Reset(dst io.Writer)

func (w *Writer) EncodeNil() (err error)
func (w *Writer) EncodeBool(value bool) (err error)
func (w *Writer) EncodeInt64(value int64) (err error)
func (w *Writer) EncodeUint64(value uint64) (err error)
func (w *Writer) EncodeFloat32(value float32) (err error)
func (w *Writer) EncodeFloat64(value float64) (err error)
func (w *Writer) EncodeString(value string) (err error)
func (w *Writer) EncodeBytes(value []byte) (err error)
func (w *Writer) EncodeStringFrom(length uint64, src io.Reader) (n uint64, err error)
func (w *Writer) EncodeBytesFrom(length uint64, src io.Reader) (n uint64, err error)
func (w *Writer) EncodeArrayHeader(count uint32) (err error)
func (w *Writer) EncodeMapHeader(count uint32) (err error)

func NewDecoder(src io.Reader) (out *Decoder)
func (d *Decoder) Decode(value any) (err error)
func (d *Decoder) Close() (err error)

type Decodee interface {
    DecodeMessagePack(Reader) error
}

func NewReader(src io.Reader) (out *Reader)
func (r *Reader) Reset(src io.Reader)
func (r *Reader) SetLimits(limits ReaderLimits) (err error)

func (r *Reader) DecodeNil() (err error)
func (r *Reader) DecodeBool() (value bool, err error)
func (r *Reader) DecodeInt64() (value int64, err error)
func (r *Reader) DecodeUint64() (value uint64, err error)
func (r *Reader) DecodeFloat32() (value float32, err error)
func (r *Reader) DecodeFloat64() (value float64, err error)
func (r *Reader) DecodeString() (value string, err error)
func (r *Reader) DecodeBytes() (value []byte, err error)
func (r *Reader) DecodeStringTo(dst io.Writer) (n uint64, err error)
func (r *Reader) DecodeBytesTo(dst io.Writer) (n uint64, err error)
func (r *Reader) DecodeArrayHeader() (count uint32, err error)
func (r *Reader) DecodeMapHeader() (count uint32, err error)
func (r *Reader) Skip() (err error)
```

`ReaderLimits` has its own `reader_limits.go` file. Its entire public
shape is:

```go
type ReaderLimits struct {
    MaxPayloadBytes uint64
    MaxSkipDepth    uint32
    MaxSkipValues   uint64
    MaxSkipBytes    uint64
}
```

No configuration framework, exported option functions, error accessor, or flush
is included. Each operation reports its own error. Writer and Reader retain
Reset; Reader also exposes SetLimits. Encoder exposes only Encode and Close;
Decoder exposes only Decode and Close.

## Custom value encoding

- `Encode` checks closure and stream readiness before dispatching to
  `EncodeMessagePack(Writer)` or reflecting the value. Unsupported root kinds
  return `ErrUnsupportedType` directly without I/O or a terminal failure.
- The concrete Writer passed by value shares output, payload buffering, and error
  state with its Encoder. Typed nil Encodee pointers retain normal Go method behavior.
- Hook errors are wrapped with encode context and retained as terminal failures.
  A stored Writer I/O failure takes precedence, including when the hook ignores
  it or returns a different error. Later calls return the stored error without
  invoking the hook or performing I/O.
- Close releases the output, prevents subsequent Encode calls with `ErrClosed`,
  and returns the stored encoding failure, if any. Repeated Close calls return
  the same result. Close never flushes or closes the supplied output.
- A zero-value Encoder or NewEncoder(nil) returns ErrUninitialized from Encode.
  Encoder has no Reset; create a new Encoder to attach or replace an output.

## Custom value decoding

- Decode checks closure and stream readiness before dispatching to
  DecodeMessagePack(Reader) or reflecting a non-nil pointer target. Invalid targets
  and unsupported root kinds return ErrUnsupportedType directly without I/O or
  a terminal failure.
- Initialized Reader copies share input, EOF, limits, buffering, and failure state.
  A hook can configure ReaderLimits through SetLimits. Typed nil Decodee pointers
  retain normal Go method behavior.
- Returned hook errors preserve schema context, including an incomplete-message
  error replacing EOF before a required child. Reader errors returned unchanged
  keep their identity; other hook errors gain decode context. All hook failures
  are terminal. If the hook returns nil after a Reader failure, that failure is
  still returned. Later Decode calls perform no I/O or hook dispatch.
- Close releases the input and makes later Decode calls return ErrClosed. It
  returns any stored decoding failure and never drains or closes the input.
  Repeated Close calls return the same result.
- Zero-value Decoders and NewDecoder(nil) are unattached. Decoder has no Reset;
  construct a new Decoder to attach or replace input.

## Reflection fallback

Reflection supports Go booleans, integer kinds (including uintptr), floats,
strings, pointers, interfaces, arrays, slices, maps, and structs. Named types use
their underlying kind. Hooks take priority in nested values too; addressable
values can use pointer-receiver hooks. Top-level typed nil hook receivers retain
ordinary Go method behavior. Missing nested decode pointers are allocated.

Codec plans are cached by reflect.Type across Encoder and Decoder instances.
Each type's dispatch functions, hook eligibility, and struct field metadata are
initialized once, with synchronized cache publication. Plans are retained for
the process lifetime and do not retain application values or stream state.
Recursive child types are resolved through the cache during traversal.
Encoding plans hold child plan references so static struct fields, pointers,
array/slice elements, and map keys/values do not repeat global cache lookups.
Child initialization stays lazy to support recursive types. Struct headers and
keys are encoded once with the typed Writer, with the map header fused into the
first key prefix. Cached prefixes use bounded writes of at most 32 KiB.
Encoder owns and reuses its reflection context and Writer scratch across calls.
Field values remain reflective; no generator or additional build step is used.

Structs encode maps with exact exported Go field names in declaration order;
embedded fields keep their own names. Unexported fields and tags are ignored.
Unknown fields are consumed with Skip, missing fields retain their values, and
the last duplicate field wins. Map iteration order is unspecified. Arrays require
exact lengths. Numeric conversions retain typed Reader rules and check the
target Go type's range.

Byte slices use binary and nil byte slices encode empty binary. Other nil slices,
maps, pointers, and interfaces encode nil. Wire nil clears nullable targets.
Reflected byte-slice fields and targets reuse sufficient destination capacity;
larger payloads allocate new storage. Empty binary preserves existing capacity.
Length and payload limits are checked before reslicing or modifying storage, even
when the destination has ample capacity. Reusing a target can overwrite earlier
slice aliases, and a failed payload read can leave a partially overwritten
buffer. Input and destination storage must be disjoint. Typed Reader.DecodeBytes
continues to return fresh owned results independent of subsequent reads.
An empty interface materializes bool, uint64 for positive fixints and unsigned
wire encodings, int64 for signed wire encodings, float32/float64, string, []byte,
[]any, map[any]any, or nil. An interface holding a non-nil pointer decodes into
that pointer. Non-comparable map keys fail with ErrTypeMismatch.

Reflection is capped at 256 recursive calls, counting pointer/interface wrappers.
Each decode has 1048576 visits, including wrappers, keys, and values; unknown
fields additionally use Skip's own limits. Container child counts are checked
against the remaining visit budget before traversal. Slices and maps grow only
as children arrive. Owned payload limits still apply. These traversal limits are
fixed; hook code retains responsibility for its own traversal. ErrLimitExceeded
also covers reflection limits, including cyclic encoding input reaching the
depth cap. Missing reflected children convert Reader EOF to io.ErrUnexpectedEOF;
EOF before a new root remains io.EOF. Traversal failures are terminal and may
leave partial output or target updates.

## Initialization, ownership, and reset

The following reset rules apply to Writer and Reader.

- A zero-value codec is unattached. A constructor attaches its argument and starts
  with no terminal failure. `NewReader` uses the defaults below. Constructors
  return usable codec pointers even when given a nil interface; such instances
  remain unattached until reset with a non-nil stream.
- Encoding/decoding/skipping on an unattached instance returns
  `ErrUninitialized` directly, performs no I/O, and does not latch a failure.
  `SetLimits` is allowed on an unattached Reader.
- `Reset` clears the terminal error, pending EOF, per-operation progress, and
  traversal state, and attaches the new stream. A nil interface leaves the codec
  unattached. Reader limits survive reset; resetting a never-configured zero
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
  safety. Initialized Writer copies share their output, payload buffer, and failure
  state. Reader copies share input, EOF, limits, payload buffering, and failure
  state. Reset or SetLimits through one copy affects all copies. Zero-value copies are
  independent until initialized. Encoder and Decoder must not be copied after
  first use. Shared instances must not be used concurrently.
- `DecodeString` and `DecodeBytes` return owned results stable across subsequent
  calls and reset. Failed materializing reads return `""` or nil. Successful empty
  binary returns a non-nil empty slice. There are no borrowed stream results.
  In-memory encoding does not retain input after the method returns.

## Limits and bounded memory

Each zero field in a `ReaderLimits` argument selects that field's default.
Zero never means unlimited. `SetLimits` validates and replaces the complete
configuration atomically, without I/O; it is allowed before initialization and
after terminal failure. It cannot clear a terminal failure. Invalid configuration
returns `ErrInvalidArgument` directly and leaves all previous limits and
stream state unchanged. There is no partial update.

| Field | Default | Valid effective values | Applies to |
| --- | --- | --- | --- |
| `MaxPayloadBytes` | 16 MiB (`16777216`) | 1–`4294967295` bytes | Each owned string/binary result and reflected binary payload, including reused storage. |
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
output, preserving rejection of oversized payloads.

For a positive declared length, emit the smallest family header, then read and
write exactly that many payload bytes on success. Never probe beyond the declared
length, even to check EOF. Extra source bytes remain unread. Early source EOF is
`io.ErrUnexpectedEOF`, including when no payload byte was supplied after the
header. Source failure after a header is terminal; the escaped prefix stays written.
Source bytes accompanied by `io.EOF` complete successfully if they supply exactly
the remaining declared payload. Writer source EOF is not retained across separate
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
| Bytes with `io.EOF`, enough for the entire requested value | Complete successfully; Reader retains pending EOF for the next operation without an extra underlying read. Writer transfers retain no source EOF across calls. |
| Bytes with `io.EOF`, enough for a segment but not the entire requested value | Use the segment; the next required read observes EOF and the operation fails as incomplete. |
| `io.EOF` before any byte of a new typed operation/Skip root | Return context-wrapped `io.EOF`. |
| `io.EOF` after the current operation began, including a missing Skip child | Return context-wrapped `io.ErrUnexpectedEOF`; no `io.EOF` match. |
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

The codec sentinels, each with
GoDoc, are `ErrUninitialized` for no attached stream, `ErrInvalidArgument` for
invalid limits or required nil transfer interfaces, `ErrLimitExceeded` for consumed
values exceeding reader or reflection traversal limits, and `ErrClosed` for a closed Encoder
or Decoder. ErrUnsupportedType also covers unsupported Go kinds and invalid
decode targets.
Error strings are not part of the contract.

Wrap external source/destination errors with operation and side context using
`%w`, preserving `errors.Is`. This includes EOF, unexpected EOF, short writes,
and no progress. Return codec/state sentinels directly. In particular, stream
unexpected EOF does **not** also match clean `io.EOF`.

Typed Writer/Reader operations first return an existing terminal failure, then
check attachment, then validate pre-I/O arguments in the order described above.
The following table defines whether subsequent value operations can proceed:

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
follows the wire classification: reserved `0xc1`, then extension tags, then supported-family
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

For an HTTP response, the caller constructs a Reader directly on `resp.Body`,
sets an appropriate materialization cap for small fields, and traverses a known
schema using typed calls. It copies a large binary/string directly to a sink with
`DecodeBytesTo`/`DecodeStringTo`. The caller closes the body and supplies request
context/deadlines. S05 proves transfer bounds; S06 proves complete typed messages;
S08 proves local incremental HTTP, truncation, cancellation, and sink failures.

For files, the caller opens a file, optionally wraps it in `bufio.Writer`, and
encodes headers and children incrementally. A known-length payload streams from
a reader through `EncodeBytesFrom`/`EncodeStringFrom`. The caller checks encode,
flush, and close errors separately; durability requires caller-owned file policy.
Reopening with a Reader permits incremental validation without a whole-file
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
