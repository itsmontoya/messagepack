# MessagePack PoC contract

## Status and environment

Approved by the maintainer on 2026-11-01. In this session, the maintainer explicitly
authorized agents to implement production code for tasks 1–11 in
[`TODO.md`](../TODO.md), overriding the production-code restriction in
[`AGENTS.md`](../AGENTS.md) for this work, and approved this contract, including
the Go 1.27.2 baseline. The repository policy itself remains unchanged.

The module is `github.com/itsmontoya/messagepack`, with package name `messagepack`.
The minimum Go version is **1.27.2**, matching the installed
`go1.27.2 darwin/arm64` toolchain inspected for task 1. No third-party dependencies
or additional tooling are included. The TODO's API and behavior proposals were
approved without changes; the concrete Go version is the environment-based addition.

This is a supported subset of MessagePack. The wire-format reference is the
[official MessagePack specification](https://github.com/msgpack/msgpack/blob/master/spec.md).
The API, conversion rules, ownership rules, and error precedence below are project
choices approved from the TODO.

## Scope

Use typed package functions from the standard library only. Support nil, booleans,
signed and unsigned 64-bit integers, float32 and float64, strings, binary payloads,
and array/map headers. Callers write and read container contents explicitly.

Extensions, including timestamps, are unsupported. Streaming I/O, generic
marshal/unmarshal, encoder/decoder objects, interfaces, configuration frameworks,
reflection, `any` dispatch, struct tags and automatic mapping, code generation,
recursive skipping or validation, map sorting, pooling, concurrency machinery,
`unsafe`, assembly, and speculative fast paths are deferred.

## Public API contract

These signatures document the eventual surface across tasks 2–7. Task 1 introduces
only package documentation and the five sentinel errors; it adds no placeholder
codec functions.

```go
func AppendNil(dst []byte) (out []byte)
func ReadNil(src []byte) (n int, err error)

func AppendBool(dst []byte, value bool) (out []byte)
func ReadBool(src []byte) (value bool, n int, err error)

func AppendInt64(dst []byte, value int64) (out []byte)
func AppendUint64(dst []byte, value uint64) (out []byte)
func ReadInt64(src []byte) (value int64, n int, err error)
func ReadUint64(src []byte) (value uint64, n int, err error)

func AppendFloat32(dst []byte, value float32) (out []byte)
func AppendFloat64(dst []byte, value float64) (out []byte)
func ReadFloat32(src []byte) (value float32, n int, err error)
func ReadFloat64(src []byte) (value float64, n int, err error)

func AppendString(dst []byte, value string) (out []byte, err error)
func AppendBytes(dst []byte, value []byte) (out []byte, err error)
func ReadString(src []byte) (value string, n int, err error)
func ReadStringBytes(src []byte) (value []byte, n int, err error)
func ReadBytes(src []byte) (value []byte, n int, err error)

func AppendArrayHeader(dst []byte, count uint32) (out []byte)
func ReadArrayHeader(src []byte) (value uint32, n int, err error)
func AppendMapHeader(dst []byte, count uint32) (out []byte)
func ReadMapHeader(src []byte) (value uint32, n int, err error)
```

Callers retain the returned append slice. Reads consume one value at the start of
`src`, return its byte count in `n`, and leave trailing values for the caller to
read from `src[n:]`. Reads never modify input. Every read failure returns the Go
zero value and `n == 0`; a failed slice read returns a nil slice.

## Buffer ownership

- Appends preserve the destination prefix and reuse available capacity. Growth may
  allocate. A returned append error leaves both the destination slice and its
  backing storage unchanged.
- Payload and destination storage must be disjoint for append operations. Support
  for overlapping storage is outside the contract.
- `ReadBytes` and `ReadStringBytes` return borrowed views of the source. Source
  mutation changes the view, and retaining a view retains the backing array.
  Copy a view before independent ownership or reuse of its source storage.
- Borrowed views have capacity equal to length. Appending to a view cannot
  overwrite following encoded values, but existing elements remain mutable.
- `ReadString` returns an owned string through safe conversion. Its contents do
  not change when the source is reused; allocating for this ownership is acceptable.
- Strings preserve bytes exactly, including invalid UTF-8, embedded NUL bytes, and
  multibyte text. Neither string reader validates or normalizes UTF-8.
- `AppendBytes` treats a nil slice as empty binary. Encode nil with `AppendNil`.
  Empty borrowed views may be nil or non-nil; distinguish success from failure
  through `err`.

## Encoding and conversions

Multi-byte fields use big-endian order. Append operations choose the smallest
integer, payload header, or container header allowed by these rules. Readers also
accept wider valid encodings. Strings and binary remain distinct wire families;
nil is never coerced to a zero or empty value.

### Integers

Nonnegative signed values encode identically to unsigned values. Integer encoders
use these ranges:

| Value range | Encoding |
| --- | --- |
| 0–127 | positive fixint |
| 128–255 | uint8 |
| 256–65535 | uint16 |
| 65536–4294967295 | uint32 |
| 4294967296–18446744073709551615 | uint64 |
| -32–-1 | negative fixint |
| -128–-33 | int8 |
| -32768–-129 | int16 |
| -2147483648–-32769 | int32 |
| -9223372036854775808–-2147483649 | int64 |

Both integer readers accept every integer wire form if the decoded value fits
the requested Go type. `ReadInt64` rejects values above 9223372036854775807;
`ReadUint64` rejects negative values. These failures return `ErrOverflow`.
Integer/float coercion is unsupported.

### Floating point

Float appends preserve their declared wire width and bit pattern, including signed
zero, infinities, and NaNs. `ReadFloat32` accepts only float32. `ReadFloat64`
accepts float64 and widens float32 with Go's numeric conversion. Widening preserves
finite values, signed zero, and infinities; a NaN remains NaN without a promise
about its widened payload bits. Reading float64 as float32 returns
`ErrTypeMismatch`, even when its value would fit. Same-width reads preserve bits.

### Payloads and containers

String lengths are byte lengths. Payload lengths above 4294967295 return
`ErrOverflow` before any append mutation. Use fixstr through length 31, str8/bin8
through 255, str16/bin16 through 65535, and str32/bin32 for larger wire lengths.
Binary has no fixed-length compact header.

Container counts are `uint32`: arrays count elements; maps count key/value pairs.
Use fixarray/fixmap through count 15, array16/map16 through 65535, and
array32/map32 above that. Header appends need no count-validation error return.
Header reads return only the count and header consumption; a complete header with
missing or malformed children still succeeds. They do not allocate containers,
multiply map counts, or walk children.

Check advertised lengths against available input before slicing, adding offsets,
or converting to `int`, including on 32-bit platforms. Never allocate from an
untrusted advertised length alone.

## Errors and precedence

The package's `errors.go` exports these direct sentinel errors, each with GoDoc.
Callers inspect errors with `errors.Is`; error message text is not a contract.
Ordinary contract failures do not allocate formatted errors.

| Sentinel | Meaning |
| --- | --- |
| `ErrTruncated` | Input is empty, or an expected value's header or payload is incomplete. |
| `ErrInvalidFormat` | The leading tag is the reserved byte `0xc1`. |
| `ErrUnsupportedType` | The leading tag belongs to an extension format. |
| `ErrTypeMismatch` | The tag is supported but not accepted by the requested reader. |
| `ErrOverflow` | A complete numeric value cannot fit the requested type, or an append payload exceeds the wire length limit. |

Every reader applies this order:

1. Empty input returns `ErrTruncated`.
2. Tag `0xc1` returns `ErrInvalidFormat`.
3. Extension tags `0xc7`–`0xc9` and `0xd4`–`0xd8` return
   `ErrUnsupportedType`, without requiring a complete extension value.
4. Another supported but unexpected family returns `ErrTypeMismatch`, even when
   that value is incomplete. Readers need not parse that family's payload.
5. An expected family with an incomplete header or payload returns `ErrTruncated`.
6. A complete expected numeric value outside the requested range returns
   `ErrOverflow`.

For example, `ReadBool` on a lone uint16 tag returns `ErrTypeMismatch`, while
`ReadInt64` on that same input returns `ErrTruncated`. A complete negative integer
read as unsigned returns `ErrOverflow`. All failures consume zero bytes.

## Caller responsibilities and performance targets

Callers own container traversal, ordering, expected field names, schema validation,
and checking full-message consumption. Header success does not validate a message.
Borrowed storage must not be reused while an independent consumer needs its
contents; this API supplies no synchronization for shared mutable buffers.

Target zero heap allocations for successful primitive reads, borrowed payload
reads, header reads, and appends with sufficient destination capacity. These are
measurement targets for task 10, not verified guarantees. Destination growth and
owned string reads may allocate. Prefer clarity when avoiding an allocation would
complicate ownership or implementation.

## Task 1 handoff

- Recorded the maintainer-approved contract and session authorization for tasks
  1–11; confirmed the local Go 1.27.2 toolchain.
- Added `go.mod`, package GoDoc in `doc.go`, and the five documented sentinel errors
  in `errors.go`. No codec placeholders or third-party dependencies were added.
- Applied `gofmt` to the Go files. `go build ./...`, `go test ./...` (no test files
  yet), and `go vet ./...` pass with `GOCACHE=/private/tmp/messagepack-go-cache`.
- Task 1 is complete with no unresolved contract issues. Tasks 2–7 introduce the
  codec functions listed above in their prescribed order.
- The title-only README remains scheduled for expansion in task 11.
