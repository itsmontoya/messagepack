# MessagePack PoC

A small, typed MessagePack codec for Go 1.27.2 or later, using only the standard
library. Callers append into their own buffers and decode one value at a time.
The package supports nil, booleans, signed and unsigned 64-bit integers,
float32/float64, strings, binary payloads, and array/map headers.

This is a supported subset of MessagePack. Extensions, including timestamps,
are rejected. See the [approved contract](docs/poc-contract.md) for exact
signatures, conversions, wire behavior, and ownership rules.

## Use a buffer and retain a payload

```go
package main

import (
	"fmt"
	"log"

	"github.com/itsmontoya/messagepack"
)

func main() {
	var (
		dst     = make([]byte, 0, 64)
		payload = []byte{1, 2, 3}
		view    []byte
		n       int
		err     error
	)
	dst, err = messagepack.AppendBytes(dst, payload)
	if err != nil {
		log.Fatal(err)
	}
	view, n, err = messagepack.ReadBytes(dst)
	if err != nil {
		log.Fatal(err)
	}
	if n != len(dst) {
		log.Fatal("unexpected trailing bytes")
	}
	// Retain a copy before reusing the encoded source storage.
	owned := append([]byte(nil), view...)
	fmt.Printf("payload: %x\n", owned)
	dst = messagepack.AppendBool(dst[:0], true)
	fmt.Printf("reused buffer: %x, owned payload: %x\n", dst, owned)
}
```

Output:

```text
payload: 010203
reused buffer: c3, owned payload: 010203
```

Always keep the slice returned by an append. Appends preserve the existing prefix
and reuse capacity when available; growth may allocate. Payload storage and the
destination backing array must be disjoint. A returned append error preserves
both the original destination and its backing storage.

## Typed operations

| Value | Append | Read |
| --- | --- | --- |
| Nil | `AppendNil` | `ReadNil` |
| Boolean | `AppendBool` | `ReadBool` |
| Signed/unsigned integer | `AppendInt64`, `AppendUint64` | `ReadInt64`, `ReadUint64` |
| Floating point | `AppendFloat32`, `AppendFloat64` | `ReadFloat32`, `ReadFloat64` |
| String | `AppendString` | `ReadString`, `ReadStringBytes` |
| Binary | `AppendBytes` | `ReadBytes` |
| Container header | `AppendArrayHeader`, `AppendMapHeader` | `ReadArrayHeader`, `ReadMapHeader` |

Readers return a value, consumed byte count `n`, and error; `ReadNil` returns
only `n` and error. A successful read leaves trailing bytes for the caller to
process from `src[n:]`. Reads never modify input. On failure, the value is zero
(nil for slice results) and `n == 0`.

Integer encoders choose the smallest representation; nonnegative signed values
encode identically to unsigned values. Readers accept wider integer encodings
when the value fits. Negative-to-unsigned and overflowing unsigned-to-signed
reads fail. Float appends preserve width and bits. `ReadFloat64` also widens
float32; `ReadFloat32` rejects float64. Integer/float coercion is unsupported.

## Ownership and containers

`ReadBytes` and `ReadStringBytes` return borrowed views with capacity equal to
length. Mutating the source changes the view, and retaining the view retains the
backing array. Copy before independent ownership or source reuse. Appending to a
view cannot overwrite the following encoded value, but its existing elements
remain mutable. Shared mutable buffers require caller synchronization.

`ReadString` returns an owned string through safe conversion. It remains unchanged
when the source is reused; copying may allocate. String append/read APIs preserve
bytes exactly, including invalid UTF-8 and embedded NULs, without validation or
normalization. Strings and binary are distinct wire families. `AppendBytes(dst, nil)`
encodes empty binary; use `AppendNil` to encode nil. For empty borrowed
views, use `err` to distinguish success rather than relying on slice nilness.

Array counts are elements; map counts are key/value pairs. Counts use `uint32`.
Header reads validate only the header: they succeed even when child values are
missing or malformed. Callers traverse contents, validate field names, ordering,
counts and schemas, and check complete-message consumption. No allocation follows
solely from an advertised payload length or container count.

The runnable [examples](example_test.go) and [integration fixtures](integration_test.go)
encode and decode a six-field message with a nested array, show buffer reuse and
concatenated messages, and copy borrowed data. Their fixed schema rejects unknown,
duplicate, or reordered fields; unknown-field skipping is deferred.

## Errors

Inspect the direct sentinel errors with `errors.Is`; message text is not a contract.

| Error | Meaning |
| --- | --- |
| `ErrTruncated` | Empty input or an incomplete expected header/payload. |
| `ErrInvalidFormat` | Reserved tag `0xc1`. |
| `ErrUnsupportedType` | Extension tag, including timestamps. |
| `ErrTypeMismatch` | A supported family rejected by the requested reader. |
| `ErrOverflow` | Unrepresentable integer conversion or append payload above the wire length limit. |

Empty input is truncated. Reserved and extension tags are classified before type
mismatches. An unexpected supported family is a mismatch even if incomplete;
readers do not parse its payload. For an expected family, truncation takes
precedence over numeric overflow. All read errors consume zero bytes.

## Verification and limits

```sh
go build ./...
go test ./...
go vet ./...
go test -run '^Example' -v ./...
go test -race ./...
```

The [fuzz report](docs/poc-fuzzing.md) records separate runs of all 17 targets for
at least 30 seconds each. Linux/386 tests cross-compiled successfully; they were
not executed because a compatible runtime was unavailable. Passing race checks
does not authorize concurrent mutation of shared buffers.

The [benchmark report](docs/poc-benchmarks.md) includes five samples for each of
42 cases and the raw output. The measured successful primitive, borrowed-payload,
and header reads and reused appends allocated zero. Owned strings and fresh-buffer
growth allocate as documented; these are scoped measurements, not a blanket
zero-allocation promise.

Streaming I/O, generic marshal/unmarshal, reflection, automatic struct mapping,
struct tags, code generation, recursive skipping/validation, map sorting, pooling,
concurrency machinery, unsafe conversions, assembly, and speculative fast paths
are deferred. The PoC exposes typed package functions without encoder/decoder
objects or a configuration framework.

## Contributions

[AGENTS.md](AGENTS.md) reserves production code for human contributors and limits
AI assistance. This PoC was implemented under the maintainer's explicit
session-specific authorization for [TODO tasks 1–11](TODO.md), recorded in the
approved contract. That authorization does not change the standing policy.
Follow [STYLEGUIDE.md](STYLEGUIDE.md) for comments, documentation, and tests.
