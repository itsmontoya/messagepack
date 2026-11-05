# MessagePack

A small, typed MessagePack codec for Go 1.27.2 or later, using only the standard
library. `Writer` writes typed values to an `io.Writer`; `Reader` reads typed values
from an `io.Reader`. `Encoder` and `Decoder` prefer custom `Encodee` and `Decodee`
hooks and use reflection for ordinary Go values.
Process HTTP response bodies, file datasets, and consecutive messages without
holding an entire encoded object in memory. Known-length payload transfers use
bounded codec memory; convenient owned payload reads have finite size limits.

The package supports nil, booleans, signed/unsigned 64-bit integers,
float32/float64, strings, binary, array/map headers, and bounded `Skip`.
Extensions, including timestamps, are unsupported. The
[approved streaming contract](docs/streaming-contract.md) records the exact API
and behavior. [In-memory workflows](#encode-and-decode-in-memory) use the same typed API.

## Encode custom values

`Encoder` exposes `Encode(value any)` and `Close()`. Values implementing this
interface control their schema; other supported values use reflection:

```go
// Encodee defines the encoding hook accepted by Encoder.
type Encodee interface {
	EncodeMessagePack(Writer) error
}
```

Implement an explicit schema with the typed `Writer` methods:

```go
// Record describes the one-field map used by the encoding example.
type Record struct {
	ID uint64
}

// EncodeMessagePack writes Record's one-field map schema.
func (r Record) EncodeMessagePack(w messagepack.Writer) (err error) {
	if err = w.EncodeMapHeader(1); err != nil {
		return err
	}

	if err = w.EncodeString("id"); err != nil {
		return err
	}

	return w.EncodeUint64(r.ID)
}
```

Pass the record to `messagepack.NewEncoder(dst).Encode(record)`.
The [executable example](encoder_example_test.go) keeps the Encoder and calls
`Close` after encoding. `Close` is idempotent, prevents further encoding with
`ErrClosed`, and returns any stored encoding failure. It neither flushes nor closes
the supplied output. A zero-value Encoder or `NewEncoder(nil)` is unattached;
construct a new Encoder to attach an output.

The by-value `Writer` passed to `EncodeMessagePack` shares its output, payload
buffer, and failure state with the Encoder. Hook failures are terminal, including
errors returned after partial output. Unsupported root kinds perform no I/O and
leave a healthy Encoder usable. Typed nil pointers implementing `Encodee` receive
the normal Go method call.

## Decode custom values

`Decoder` exposes `Decode(value any)` and `Close()`. Values implementing this
interface control their schema; other targets must be non-nil pointers to
supported Go values. Invalid targets and unsupported root kinds return
`ErrUnsupportedType` without reading input or making the Decoder terminal.

```go
// Decodee defines the decoding hook accepted by Decoder.
type Decodee interface {
	DecodeMessagePack(Reader) error
}
```

Targets usually implement this method on a pointer receiver. Pass the target to
`messagepack.NewDecoder(src).Decode(&record)`; the
[executable example](decoder_example_test.go) validates a map schema and fills its
record through typed Reader methods.

The concrete Reader passed by value shares input, EOF, limits, payload buffering,
and failure state with the Decoder. Call `Reader.SetLimits` in the hook when a
schema requires custom limits. Hook errors preserve schema context and become
terminal. If a hook ignores a Reader failure, Decode still returns that failure.

`Close` is idempotent, prevents further decoding with `ErrClosed`, and returns any
stored failure. It neither drains nor closes the supplied reader. A zero-value
Decoder or `NewDecoder(nil)` is unattached; construct a new Decoder to attach input.
Typed nil pointers implementing Decodee receive the normal Go method call.

## Reflect ordinary Go values

Hooks take priority at the root and in nested values. Without a hook, reflection
supports booleans, all signed and unsigned integer kinds, float32/float64, strings,
pointers, interfaces, arrays, slices, maps, and structs. Named types use their
underlying kind. Numeric reads retain the typed Reader's conversion rules and
reject overflow into smaller Go integers.

Reflection plans are cached by `reflect.Type` across Encoder and Decoder
instances. Field metadata, codec functions, and hook eligibility are initialized
once per type and reused. Plans link directly to child handlers. Encoding also
caches wire bytes for struct headers and keys, combining adjacent prefixes.
The Encoder and Decoder own reusable scratch and traversal state. Field values
still use reflection; there is no generator or additional build step. The synchronized
cache retains type metadata for the process lifetime and never stores application
values or stream state.
The [allocation measurements](docs/reflection-allocations.md) cover warmed
scalar, mixed, and nested structs, including zero-allocation encoding for the
measured pointer inputs. The [decoder and string follow-up](docs/decoder-string-optimizations.md)
covers zero-allocation warmed scalar decoding and remaining owned-result allocations.

```go
var (
    output  bytes.Buffer
    value   = struct{ ID int }{ID: 42}
    decoded struct{ ID int }
    err     error
)

if err = messagepack.NewEncoder(&output).Encode(value); err != nil {
    panic(err)
}

if err = messagepack.NewDecoder(&output).Decode(&decoded); err != nil {
    panic(err)
}
```

Structs use maps with exact exported Go field names, in declaration order.
Unexported fields are ignored; embedded fields keep their own names. Struct tags
are not interpreted. Decoding skips unknown fields with `Reader.Skip`, leaves
missing fields unchanged, and lets the last duplicate field win. Ordinary map
iteration order is unspecified; fixed arrays require an exact element count.

Byte slices use binary, including empty binary for a nil byte slice, matching
`Writer.EncodeBytes`. Other nil slices, nil maps, nil pointers, and nil interfaces
encode nil; non-nil slices and arrays use arrays. Wire nil clears pointer, slice,
map, and interface targets. Decode allocates missing nested pointers as needed.

Reflected byte-slice fields and targets reuse their backing arrays when capacity
is sufficient, and allocate larger storage otherwise. Empty binary retains
capacity; wire nil clears the slice. Decoding into the same target can overwrite
earlier slice aliases, including a partial prefix on failure. Copy bytes to retain
an earlier value, and keep input storage separate from destination buffers.
The typed `Reader.DecodeBytes` method always returns fresh owned storage.

An empty interface receives bool, uint64 for unsigned wire integers (including
positive fixints), int64 for signed wire integers, float32/float64, string, []byte,
[]any, map[any]any, or nil. An interface already holding a non-nil pointer decodes
into that pointer. Map keys must be comparable Go values.

Reflection has a fixed recursion limit of 256 calls and a per-decode budget of
1048576 visits, including pointer/interface traversal. String and binary results
use the Reader's payload cap. Container storage grows as children arrive, rather
than allocating from an advertised count alone. Limit failures return
`ErrLimitExceeded`; cyclic inputs eventually reach the recursion limit.
Failures encountered during traversal are terminal and may leave partial output
or target updates. Missing required children report `io.ErrUnexpectedEOF`;
EOF before a new root remains `io.EOF`.

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

	d := messagepack.NewReader(response.Body)
	n, err = d.DecodeBytesTo(dst)
	return n, err
}
```

For a structured response, read its map/array header, decode small fields with
typed calls, and send large fields to `DecodeBytesTo` or `DecodeStringTo`.
The executable `ExampleReader_httpResponse` in
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
		e        = messagepack.NewWriter(buffered)
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

Reopen with `NewReader`, optionally on a `bufio.Reader`, and process each array
element before reading the next. `ExampleWriter_file` in
[the executable examples](stream_example_test.go) and the
[file workflow tests](stream_file_workflow_test.go) write and validate 100000
integers plus an 8 MiB + 7-byte payload without collecting either dataset.
Encode success means the writer accepted bytes. Flush, close, and file durability
remain caller responsibilities; a failed write may leave a partial file.

## Typed operations and containers

| Value | Writer method | Reader method |
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
encode identically to unsigned values. Readers accept wider integer encodings
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
String reads up to 32 KiB reuse bounded staging storage after warm-up; the returned
string still owns its bytes. Larger strings use temporary payload-sized staging.
In-memory string writes grow reusable scratch to the required size, up to 32 KiB.
`Reader.SetLimits` configures their cap and independent per-call Skip budgets:

| `ReaderLimits` field | Default | Meaning |
| --- | --- | --- |
| `MaxPayloadBytes` | 16 MiB | Each owned string/binary result; maximum configurable cap is uint32 maximum. |
| `MaxSkipDepth` | 64 | Nested container levels; hard maximum 256, including empty containers. |
| `MaxSkipValues` | 1048576 | Values per Skip, including its root, containers, and each map key/value. |
| `MaxSkipBytes` | 64 MiB | Wire bytes per Skip, including tags and headers. |

For example, `d.SetLimits(messagepack.ReaderLimits{MaxPayloadBytes: 1024})`
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
| `io.ErrUnexpectedEOF` | The current header, scalar, payload, or skipped value was incomplete. |
| `ErrInvalidFormat` | Reserved tag `0xc1`. |
| `ErrUnsupportedType` | Extension tag, including timestamps, unsupported Go kind, or invalid decode target. |
| `ErrTypeMismatch` | A supported wire value incompatible with the requested decoder or Go target. |
| `ErrOverflow` | Numeric conversion overflow, encode length above uint32, or owned length above platform `int`. |
| `ErrLimitExceeded` | An owned-result, Skip, or reflection traversal limit was exceeded. |
| `ErrUninitialized` | No attached reader/writer. |
| `ErrClosed` | Encode or Decode called after its codec was closed. |
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
operation. A Reader retains that EOF for the next operation without an extra
read. Any simultaneous non-EOF read error fails the current operation, even
with complete bytes; transfers first offer those bytes to the sink. Short
nil-error writes become `io.ErrShortWrite`; 100 consecutive `(0, nil)` reads
become `io.ErrNoProgress`.

I/O failures, consumed decode errors, and clean EOF are terminal: later value
operations return the identical stored error without I/O. Pre-I/O argument or
encode-length errors are nonterminal on a healthy attached codec. Existing
failure and attachment checks precede argument checks. `SetLimits` cannot clear
a terminal failure.

`Writer.Reset` and `Reader.Reset` clear failure/pending EOF and attach a new stream,
preserving Reader limits and possibly reusing scratch. They cannot rewind, drain,
or repair a partial message: the caller supplies a known boundary. Zero-value
Writers and Readers and their constructors given nil interfaces are unattached
until reset. Typed nil pointers inside interfaces retain ordinary Go behavior;
nil codec receivers are unsupported.
Initialized Writer copies share output and failure state; Reader copies share
input, EOF, limits, and failure state. Both share their payload buffers;
Reset through one copy affects every copy. Do not copy an Encoder or Decoder after
first use. Shared instances and mutable buffers require caller synchronization;
passing race tests does not change this.

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

## Encode and decode in memory

Attach a Writer or Encoder to a `bytes.Buffer` for in-memory output. For input,
attach a Reader or Decoder to `bytes.NewReader(data)`. The
[executable examples](example_test.go) demonstrate buffer reuse and consecutive
messages. Reader payload results own their storage and survive input mutation and
reuse. Keep encoding input separate from output storage being overwritten.

The package-level `Append*` and `Read*` functions, including borrowed
`ReadStringBytes`, are removed. `ReaderLimits` replaces `DecoderLimits`, and stream
EOF errors replace the former `ErrTruncated` sentinel. The
[archived slice contract](docs/poc-contract.md) records the former API.

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

The [allocation profiling report](docs/allocation-profiling.md) records fresh
measurements after the November 4 Reader/Writer refactor, including small strings,
hooks, Skip, allocation profiles, and isolated optimization experiments. It
led to the applied short-string and Skip optimizations, recorded with
[before and after timings](docs/allocation-improvements.md).
A [follow-up profiling pass](docs/allocation-followup.md) also removes temporary
payload allocations when decoding strings of at most nine bytes.
The [CPU profiling report](docs/cpu-profiling.md) separates generic reflection
traversal from typed codec costs, compares generated tinylib codecs, and records
isolated reader fast-path experiments.
The [reader fast-path follow-up](docs/reader-cpu-optimizations.md) applies two small
successful-read optimizations and records their before/after results.

Package-level marshal/unmarshal functions, struct tags, extensions/timestamps,
code generation, map sorting, pooling, concurrency machinery, unsafe conversions,
assembly, and speculative fast paths remain deferred. Custom application schemas
use explicit typed calls.
Streaming transfers and bounded Skip are implemented.

## Contributions

[AGENTS.md](AGENTS.md) permits AI assistance with production code, refactoring,
documentation, and tests within the requested scope. Human maintainers guide
design and review. Follow [STYLEGUIDE.md](STYLEGUIDE.md) for all contributions.
