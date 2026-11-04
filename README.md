# MessagePack

A small, typed MessagePack codec for Go 1.27.2 or later, using only the standard
library. `Encoder` writes to an `io.Writer`; `Decoder` reads from an `io.Reader`.
Process HTTP response bodies, file datasets, and consecutive messages without
holding an entire encoded object in memory. Known-length payload transfers use
bounded codec memory; convenient owned payload reads have finite size limits.

The package supports nil, booleans, signed/unsigned 64-bit integers,
float32/float64, strings, binary, array/map headers, and bounded `Skip`.
Extensions, including timestamps, are unsupported. The
[approved streaming contract](docs/streaming-contract.md) records the exact API
and behavior. The [slice API](#lower-level-slice-api) is also available.

## Decode an HTTP response body

For a response containing one binary value, copy its payload directly to a sink.
The caller supplies the request context and owns body closure. This function
returns the number of payload bytes accepted by the sink, including on failure.
It decodes one value; it does not check for trailing response bytes.

```go
package example

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/itsmontoya/messagepack"
)

// FetchBinary copies one binary value from an HTTP response to dst.
func FetchBinary(ctx context.Context, client *http.Client, url string, dst io.Writer) (n uint64, err error) {
	var (
		request  *http.Request
		response *http.Response
	)
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("create request: %w", err)
	}
	response, err = client.Do(request)
	if err != nil {
		return 0, fmt.Errorf("fetch binary: %w", err)
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close response body: %w", closeErr))
		}
	}()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("fetch binary: HTTP %s", response.Status)
	}
	d := messagepack.NewDecoder(response.Body)
	n, err = d.DecodeBytesTo(dst)
	return n, err
}
```

For a structured response, read its map/array header, decode small fields with
typed calls, and send large fields to `DecodeBytesTo` or `DecodeStringTo`.
The executable `ExampleDecoder_httpResponse` in
[stream_example_test.go](stream_example_test.go) does this with a six-field map
and a generated 1 MiB + 7-byte payload. Its schema helpers are test code, not
package APIs. [HTTP integration tests](stream_http_integration_test.go) prove
incremental delivery, cancellation, deadlines, truncation, and sink failures.

## Write a file incrementally

This function writes `[integer array, binary payload]`. It generates integers one
at a time and streams exactly `length` bytes from `src`. The caller-created
buffering is flushed explicitly; file closure is checked on every exit.

```go
package example

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/itsmontoya/messagepack"
)

// WriteDataset writes generated integers and a known-length binary payload to a file.
func WriteDataset(path string, count uint32, length uint64, src io.Reader) (err error) {
	var file *os.File
	file, err = os.Create(path)
	if err != nil {
		return fmt.Errorf("create dataset: %w", err)
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close dataset: %w", closeErr))
		}
	}()
	var (
		buffered = bufio.NewWriter(file)
		e        = messagepack.NewEncoder(buffered)
	)
	if err = e.EncodeArrayHeader(2); err != nil {
		return err
	}
	if err = e.EncodeArrayHeader(count); err != nil {
		return err
	}
	for index := uint64(0); index < uint64(count); index++ {
		if err = e.EncodeUint64(index); err != nil {
			return err
		}
	}
	if _, err = e.EncodeBytesFrom(length, src); err != nil {
		return err
	}
	if err = buffered.Flush(); err != nil {
		return fmt.Errorf("flush dataset: %w", err)
	}
	return nil
}
```

Reopen with `NewDecoder`, optionally on a `bufio.Reader`, and process each array
element before reading the next. `ExampleEncoder_file` in
[the executable examples](stream_example_test.go) and the
[file workflow tests](stream_file_workflow_test.go) write and validate 100000
integers plus an 8 MiB + 7-byte payload without collecting either dataset.
Encode success means the writer accepted bytes. Flush, close, and file durability
remain caller responsibilities; a failed write may leave a partial file.

## Typed operations and containers

| Value | Encoder method | Decoder method |
| --- | --- | --- |
| Nil | `EncodeNil` | `DecodeNil` |
| Boolean | `EncodeBool` | `DecodeBool` |
| Signed/unsigned integer | `EncodeInt64`, `EncodeUint64` | `DecodeInt64`, `DecodeUint64` |
| Floating point | `EncodeFloat32`, `EncodeFloat64` | `DecodeFloat32`, `DecodeFloat64` |
| Owned string/binary | `EncodeString`, `EncodeBytes` | `DecodeString`, `DecodeBytes` |
| Known-length source | `EncodeStringFrom(length, src)`, `EncodeBytesFrom(length, src)` | — |
| Payload to writer | — | `DecodeStringTo(dst)`, `DecodeBytesTo(dst)` |
| Container header | `EncodeArrayHeader`, `EncodeMapHeader` | `DecodeArrayHeader`, `DecodeMapHeader` |
| One complete supported value | — | `Skip` |

Every encoding method returns an error. Ordinary decoders return `(value, error)`;
`DecodeNil` and `Skip` return only an error. Ordinary failed results are zero
values. Transfer methods return `(n uint64, err error)`.

Array counts mean elements; map counts mean key/value pairs. Headers use `uint32`
counts and neither allocate nor validate children. The caller traverses children
and validates field names, counts, duplicates, required fields, ordering, and
application limits. A header can succeed with every child missing.

For unknown fields, decode the key and call `Skip` once for its value. Skip walks
nested supported containers and leaves the following value untouched. It rejects
extensions, including timestamps. The
[flexible schema example](stream_flexible_schema_test.go) accepts reordered fields,
skips unknown values, and rejects duplicate names and missing required fields.
Its **16-entry map limit is application policy**, separate from the codec's
uint32 count support. The [strict schema example](stream_application_message_test.go)
retains its ordered six-field policy.

Integer encoders choose the smallest representation; nonnegative signed values
encode identically to unsigned values. Decoders accept wider integer encodings
when the value fits, rejecting negative-to-unsigned and overflowing
unsigned-to-signed conversions. Float encoders preserve width and bits.
`DecodeFloat64` also widens float32; widened NaN payload bits are unspecified.
`DecodeFloat32` rejects float64. Integer/float coercion is unsupported.

Strings preserve bytes, including invalid UTF-8 and NULs, without validation or
normalization. Strings and binary are distinct families. `EncodeBytes(nil)`
encodes empty binary; use `EncodeNil` for nil. Empty `DecodeBytes` succeeds with
a non-nil empty slice.

## Lengths, memory, and ownership

MessagePack requires payload byte lengths and container counts before contents.
Each length/count is at most `4294967295` (uint32 maximum), even though transfer
arguments and progress counts use `uint64`. Unknown-length input requires an
application chunk/sequence schema or external counting/staging. The codec does
not spool, seek back, or invent indefinite lengths. Larger datasets use multiple
values with individually valid lengths/counts; count map children with wide
arithmetic such as `uint64(count) * 2`.

`EncodeBytesFrom`/`EncodeStringFrom` consume exactly their declared byte length,
leave extra source bytes unread, and report early EOF as `io.ErrUnexpectedEOF`.
Zero length permits a nil source and writes only the empty payload header.
`DecodeBytesTo`/`DecodeStringTo` parse the wire length and require a non-nil sink,
even for empty payloads. They ignore owned-result and Skip limits.

Transfer scratch and payload I/O requests are bounded at 32 KiB. Scalar/header
scratch is fixed; Skip uses at most 256 container frames. Codec memory for these
paths is independent of total payload/object size. Caller readers, writers,
buffering, and retained application data have their own memory costs.

`DecodeString` and `DecodeBytes` return owned results stable after subsequent
calls or `Reset`. They intentionally allocate in proportion to accepted payloads.
`Decoder.SetLimits` configures their cap and independent per-call Skip budgets:

| `DecoderLimits` field | Default | Meaning |
| --- | --- | --- |
| `MaxPayloadBytes` | 16 MiB | Each owned string/binary result; maximum configurable cap is uint32 maximum. |
| `MaxSkipDepth` | 64 | Nested container levels; hard maximum 256, including empty containers. |
| `MaxSkipValues` | 1048576 | Values per Skip, including its root, containers, and each map key/value. |
| `MaxSkipBytes` | 64 MiB | Wire bytes per Skip, including tags and headers. |

For example, `d.SetLimits(messagepack.DecoderLimits{MaxPayloadBytes: 1024})`
sets a 1 KiB owned-result cap and selects defaults for every other field; check
the returned error. Zero fields always mean finite defaults. `SetLimits`
replaces the entire configuration atomically; invalid settings leave it unchanged.
Owned lengths are checked against the cap, then platform `int`, before payload
allocation/read. Raising the cap is an explicit caller memory policy.

There is no hidden read-ahead, output buffering, flushing, closing, or background
work. Callers may supply `bufio.Reader`/`bufio.Writer` and must keep using those
same wrappers for subsequent access. Encoding retains no input reference after
return. Callers own stream lifetimes, response/frame byte bounds, deadlines, and
cancellation. Codec limits cannot interrupt a blocking underlying I/O call.

## Failures, EOF, and reuse

Use `errors.Is` for sentinels and I/O identities; error text is not a contract.
External I/O errors are wrapped with operation context. Codec/state sentinels
are returned directly.

| Result | Meaning |
| --- | --- |
| `io.EOF` | No byte of the next requested value was available. |
| `io.ErrUnexpectedEOF` | The current header, scalar, payload, or skipped value was incomplete. Stream errors do not match `ErrTruncated`. |
| `ErrInvalidFormat` | Reserved tag `0xc1`. |
| `ErrUnsupportedType` | Extension tag, including timestamps; also rejected by Skip. |
| `ErrTypeMismatch` | A supported wire family rejected by the requested typed decoder. |
| `ErrOverflow` | Numeric conversion overflow, encode length above uint32, or owned length above platform `int`. |
| `ErrLimitExceeded` | A consumed header/value exceeds an owned-result or Skip limit. |
| `ErrUninitialized` | No attached reader/writer. |
| `ErrInvalidArgument` | Invalid limits or a required nil transfer interface. |

After a tag is read successfully, reserved/extension classification precedes
family mismatch. A mismatch consumes only that tag. For an expected integer
family, incomplete bytes precede conversion overflow. A consumed owned header
above its cap fails before allocation or a missing payload can cause EOF.

All four transfer counts measure **payload bytes accepted by the destination**,
excluding headers, including bytes accepted with an error. A failing chunk may
have consumed more source bytes than its sink accepted; counts do not imply
rollback or resumability. The codec stops without draining or retrying. If both
ends fail on the same bytes, the destination error takes precedence. A complete
count can accompany a failure.

Bytes accompanied by EOF complete successfully when they supply the entire
operation. A Decoder retains that EOF for the next operation without an extra
read. Any simultaneous non-EOF read error fails the current operation, even
with complete bytes; transfers first offer those bytes to the sink. Short
nil-error writes become `io.ErrShortWrite`; 100 consecutive `(0, nil)` reads
become `io.ErrNoProgress`.

I/O failures, consumed decode errors, and clean EOF are terminal: later value
operations return the identical stored error without I/O. Pre-I/O argument or
encode-length errors are nonterminal on a healthy attached codec. Existing
failure and attachment checks precede argument checks. `SetLimits` cannot clear
a terminal failure.

`Reset` clears failure/pending EOF and attaches a new stream, preserving Decoder
limits and possibly reusing scratch. It cannot rewind, drain, or repair a partial
message: the caller supplies a known boundary. Zero-value codecs and constructors
given nil interfaces are unattached until reset. Typed nil pointers inside
interfaces retain ordinary Go behavior; nil codec receivers are unsupported.
Do not copy a codec after first use or use it concurrently. Shared mutable
buffers require caller synchronization; passing race tests does not change this.

## Complete messages and persistent streams

Typed calls do not remember application-schema completeness. EOF before a
required child call is still `io.EOF` at the codec boundary; a schema decoder
must report an incomplete message, for example as `io.ErrUnexpectedEOF`.
Skip tracks completeness of its own root, including every nested child.

For a sequence, decode and process complete schema messages consecutively.
EOF before the next root ends the sequence; EOF within required children is an
error. Do not wait for connection EOF after each message on a persistent stream.

For exactly one message, validate the full schema and check for trailing bytes
only inside a finite body/file or an externally bounded frame. With a supplied
`bufio.Reader`, use that same wrapper for the check, for example `Peek(1)`:
EOF means no trailing bytes, any returned byte means trailing data, and another
error must be handled. Even an incomplete trailing value is unwanted data.
A rejected frame's remainder is the caller's responsibility; resetting the
codec does not discard it. [Executable examples](stream_example_test.go) and
[boundary tests](stream_boundary_workflow_test.go) demonstrate both policies.

## Lower-level slice API

Typed `Append*`/`Read*` functions remain available for complete in-memory input;
their exact signatures and compatibility rules are in the
[archived slice contract](docs/poc-contract.md). The original
[slice examples](example_test.go) demonstrate buffer reuse and concatenated
messages. Retain the returned append slice; appends preserve its prefix, reuse
capacity, and may allocate on growth. Payload and destination storage must be
disjoint. A returned append error preserves the original backing storage.

Slice reads never modify input, leave trailing bytes at `src[n:]`, and return
zero values with `n == 0` on failure. Empty or incomplete expected input returns
`ErrTruncated`. This zero-consumption guarantee applies to slices; stream reads
can consume a prefix on failure.

`ReadBytes` and `ReadStringBytes` borrow source storage with capacity equal to
length. Source mutation changes the view, and retaining it retains the backing
array. Copy before independent ownership or source reuse. Appending to the view
cannot overwrite following values, but its existing elements remain mutable.
`ReadString` returns an owned string. Use `err` to distinguish empty borrowed
success from failure, rather than relying on slice nilness.

## Verification and remaining scope

```sh
go build ./...
go test ./...
go vet ./...
go test -run '^Example' -v ./...
go test -race ./...
```

The [streaming fuzz report](docs/streaming-fuzzing.md) records four separate
30-second-or-longer runs on November 1–2, 2026, covering fragmented I/O, boundaries,
ownership, progress, and terminal state. These are bounded smoke runs. Linux/386
tests cross-compiled successfully; they were **not executed** because a compatible
runtime was unavailable. No cross-platform runtime or performance claim follows.

The [streaming measurement report](docs/streaming-benchmarks.md) records five
samples for each of 91 cases on November 2, 2026. Generated 1 KiB, 64 KiB, and
8 MiB transfers showed allocation bytes and retained scratch independent of
payload size. Fresh transfer codecs measured about 32 KiB plus codec storage;
prewarmed Reset transfers measured zero allocation for those successful workloads.
Owned binary/string results allocate proportionally. Synthetic endpoints,
100 ms sample targets, and process/GC noise limit inference; this is no blanket
zero-allocation promise or real file/socket throughput measurement. The
[historical slice fuzz](docs/poc-fuzzing.md) and
[allocation reports](docs/poc-benchmarks.md) preserve their separate evidence.

Generic marshal/unmarshal, reflection, `Encode(any)`/`Decode(any)`, automatic
struct mapping, struct tags, extensions/timestamps, code generation, map sorting,
pooling, concurrency machinery, unsafe conversions, assembly, and speculative
fast paths remain deferred. Application schemas use explicit typed calls.
Streaming transfers and bounded Skip are implemented.

## Contributions

[AGENTS.md](AGENTS.md) reserves production code for human contributors and limits
AI assistance. The maintainer granted scoped session authorization for
[streaming tasks S01–S11](docs/streaming-contract.md) and, separately, the
[archived PoC tasks 1–11](docs/poc-todo.md). These approvals do not change the
standing policy. Follow [STYLEGUIDE.md](STYLEGUIDE.md) for comments,
documentation, and tests.
