# MessagePack PoC

Simplicity is the north star. Build a small, readable codec that avoids allocations
through caller-owned buffers and borrowed input slices. Accept an allocation when
avoiding it would make ownership or implementation harder to understand. Performance
changes need measurements and must preserve clarity.

## How to use this plan

Complete the numbered tasks in order, one task per implementation agent. Each task
depends on all earlier tasks being complete. Give each agent this entire file,
`AGENTS.md`, `STYLEGUIDE.md`, and the current repository; task details below build on
the shared contract. Do not implement later tasks early.

**Implementation authorization:** on 2026-11-01, the maintainer explicitly authorized
agents to implement production code for tasks 1–11, overriding the production-code
restriction in `AGENTS.md` for this work, and approved
[`docs/poc-contract.md`](docs/poc-contract.md), including Go 1.27.2. This session-specific
authorization covers this plan; `AGENTS.md` itself remains unchanged. Preserve the
other repository instructions and do not silently broaden this scope.

At task 1 review, the repository contained no Go source or `go.mod`.
The README contained only the title `# Message Pack`.

For every task:

- Read the current files before editing; preserve unrelated work.
- Implement only the task's scope and include its tests and exported GoDoc.
- Use table-driven tests where cases share a structure, with explicit setup and
  literal expected wire bytes. Round trips alone cannot establish correctness.
- Follow `STYLEGUIDE.md`: one file per primary type, all its methods in that file,
  constructors immediately above their type, small functions, early returns, grouped
  `var` declarations for shared/stateful values, and no shadowing. Use named returns
  for public APIs and explicit return values. Keep local `:=` within the guide's
  narrow exceptions. Do not invent types just to organize free functions.
- Run `gofmt` on changed Go files, `go test ./...`, and `go vet ./...` once a package
  exists. Report any command that could not run and why.
- Mark only the completed task below. Hand off changed files, public API additions,
  checks and outcomes, and any unresolved issue. Do not claim unmeasured performance.
- If a task reveals a contract conflict, record it for the maintainer rather than
  silently changing an earlier task's API or broadening scope.

## Approved PoC contract

The maintainer approved these project choices for task 1. Subsequent agents must
use the recorded contract consistently.

### Scope and API shape

- Package: `messagepack`; module: `github.com/itsmontoya/messagepack`.
- Use standard-library-only package functions. No encoder/decoder objects, interfaces,
  configuration framework, reflection, `any` dispatch, or automatic struct mapping.
- Typed append functions take `dst []byte` first and return the resulting slice.
  Infallible operations return `(out []byte)`; operations with size validation return
  `(out []byte, err error)`. Callers must keep the returned slice.
- Typed read functions take `src []byte` and return `(value T, n int, err error)`.
  `n` is the number of bytes consumed. `ReadNil` returns only `(n int, err error)`.
- Reads decode one value from the start, leaving trailing bytes for the caller.
  On error, return a zero value and `n == 0`; never modify the source. Append failures
  return the original destination without modifying its backing storage.
- Expose nil, bool, signed and unsigned 64-bit integers, float32/float64, strings,
  binary payloads, and array/map headers. Callers explicitly write/read container
  contents using the same functions. Header reads validate only the header; they
  do not validate the existence or correctness of child values.
- Container counts use `uint32`, avoiding negative counts and platform-sized count
  conversions. The caller owns ordering, schema validation, and element traversal.
- No extension or timestamp support in the PoC. Reject these tags as unsupported;
  document that this is a supported subset, not a complete MessagePack implementation.
- Defer streaming I/O, generic marshal/unmarshal, struct tags, code generation,
  recursive skip/validation, map sorting, pooling, concurrency machinery, `unsafe`,
  assembly, and speculative fast paths.

### Ownership and allocations

- Append preserves the existing destination prefix and reuses capacity when available.
  Growth may allocate. Start with ordinary append operations and straightforward
  standard-library encoding helpers.
- `ReadBytes` and `ReadStringBytes` return slices borrowed from the input. Document
  that source mutation changes the view and that retaining a view retains the backing
  array. Callers must copy before independently owning or reusing the source storage.
- Limit a borrowed slice's capacity to its length, so appending to it cannot overwrite
  following encoded values. This does not make existing elements immutable.
- `ReadString` returns an owned string through safe conversion; an allocation is
  acceptable. Never use unsafe string/slice conversions to force a benchmark result.
- Treat string contents as bytes without UTF-8 validation or normalization. Preserve
  input exactly; document this choice for both string read APIs.
- A nil slice passed to `AppendBytes` means empty binary. Use `AppendNil` for nil.
  Empty binary/string views must be distinguishable from errors through `err`;
  callers must not rely on whether a zero-length view itself is nil.
- Do not promise support for payload slices overlapping the destination backing
  array; document disjoint payload/destination storage as an append precondition.
- Target zero heap allocations for successful primitive reads, borrowed payload reads,
  header reads, and append operations with enough destination capacity. Measure these
  paths explicitly; do not promise zero allocations for every API or usage pattern.

### Wire behavior and errors

Use the [official MessagePack specification](https://github.com/msgpack/msgpack/blob/master/spec.md)
as the wire-format reference. Multi-byte fields use big-endian order. The reserved
tag `0xc1` is invalid. Strings and binary payloads are distinct wire families; maps
count pairs, while arrays count elements.

- Emit the smallest integer representation. Nonnegative signed inputs use the same
  encoding as unsigned inputs. Readers accept all integer encodings when the value
  fits the requested Go type, including wider-than-necessary encodings.
- Reject negative-to-unsigned and overflowing unsigned-to-signed conversions. Do not
  coerce integers to floats, strings to binary, or nil to zero/empty values.
- Float append operations preserve their declared width. `ReadFloat32` accepts only
  float32; `ReadFloat64` accepts either float width. Do not narrow float64 implicitly.
- Choose the smallest supported payload/container header when encoding. Readers accept
  valid larger headers too. Reject payload lengths above the wire limit before writing.
- Direct sentinel errors in `errors.go`: `ErrTruncated`, `ErrInvalidFormat`,
  `ErrUnsupportedType`, `ErrTypeMismatch`, and `ErrOverflow`. Define exported GoDoc.
  Keep errors inspectable with `errors.Is`, without allocating formatted errors on
  ordinary contract failures.
- Error precedence: empty input is truncated; the reserved tag is invalid; an extension
  tag is unsupported; another supported but unexpected family is a type mismatch.
  For an expected family, incomplete headers/payloads are truncated and complete but
  unrepresentable numeric values overflow. An incomplete mismatched value still yields
  a type mismatch; readers need not parse another family's payload.
- Check lengths against available input before slicing, adding offsets, or converting
  to `int`. Never allocate memory from an untrusted advertised length. Container header
  reads return counts directly without multiplying a map count or walking children.

## Ordered tasks

### 1. Establish the contract and package foundation

- [x] Complete task 1.

**Deliverables:** `go.mod`, `doc.go`, `errors.go`, and `docs/poc-contract.md`.

- Resolve the implementation authorization noted above before writing production files.
- Record the agreed API names, argument/return types, ownership rules, supported subset,
  error precedence, and float/integer conversion rules in `docs/poc-contract.md`.
  Use the proposals above as the starting point, with any maintainer changes explicit.
- Record a concrete supported Go version in `go.mod` and the contract, based on the
  maintainer's environment. Do not add third-party dependencies or speculative tooling.
- Add concise package documentation and the agreed sentinel errors. Do not add placeholder
  codec functions that panic or return fabricated results.

**Acceptance:** the empty package builds, tests and vet run, and subsequent tasks can
implement the agreed signatures without inventing behavior.

**Progress:** the [approved contract](docs/poc-contract.md) records the API and
behavior, with Go 1.27.2 matching the installed toolchain. Added `go.mod`, package
GoDoc in `doc.go`, and the five documented sentinel errors in `errors.go`; no codec
placeholders or third-party dependencies were added. Applied `gofmt`; `go build ./...`,
`go test ./...` (no test files yet), and `go vet ./...` pass with
`GOCACHE=/private/tmp/messagepack-go-cache`.

### 2. Encode and decode nil and booleans

- [x] Complete task 2.

**Deliverables:** `nil.go`, `bool.go`, and corresponding test files.

- Implement `AppendNil`, `ReadNil`, `AppendBool`, and `ReadBool`.
- Establish the append-prefix and read-consumption conventions for later functions.
- Add only shared private tag/error helpers that reduce actual duplication. No dispatch
  tables or general parser framework.

**Acceptance:** literal wire fixtures cover nil and both booleans; tests cover empty
input, unexpected supported tags, the reserved tag, extension tags, trailing values,
preserved destination prefixes, and the error return contract. Check reuse of a
destination with sufficient capacity.

**Progress:** added `AppendNil`, `ReadNil`, `AppendBool`, and `ReadBool` with
documented prefix/consumption rules and a shared private unexpected-tag error
classifier in `errors.go`. Literal fixtures cover nil and both booleans; table
tests verify all extension tags, reserved/empty/mismatched input, zero failure
values and consumption, unchanged input, trailing values, preserved prefixes, and
destination capacity reuse. Applied `gofmt`; `go test ./...` and `go vet ./...`
pass with `GOCACHE=/private/tmp/messagepack-go-cache`. No unresolved issues.

### 3. Encode signed and unsigned integers

- [x] Complete task 3.

**Deliverables:** integer append functions in `integer.go` and table-driven tests.

- Implement `AppendInt64` and `AppendUint64` using the agreed compact encoding rule.
- Keep selection by value range explicit. Use private helpers only where they make
  the signed/unsigned relationship easier to read.
- Test both sides of every encoding-width transition, zero, negative one, and both
  types' extrema. Include boundaries around -32, -128, -32768, -2147483648, 127,
  255, 65535, and 4294967295.

**Acceptance:** expected bytes are specified independently of the implementation;
positive signed/unsigned inputs agree; destination prefixes survive; fresh and reused
destinations both work. Do not implement decoding in this task.

**Progress:** added `AppendInt64` and `AppendUint64` in `integer.go`, selecting
compact integer widths with explicit value ranges and standard-library big-endian
helpers. Literal fixtures cover both sides of every width transition, zero,
negative one, and signed/unsigned extrema. Tests verify matching nonnegative
signed/unsigned encodings, preserved prefixes, buffer growth, available-capacity
reuse, and reset destinations. No integer readers were added. Applied `gofmt`;
`go test ./...`, `go vet ./...`, and `git diff --check` pass with
`GOCACHE=/private/tmp/messagepack-go-cache`. No unresolved issues.

### 4. Decode signed and unsigned integers

- [x] Complete task 4.

**Deliverables:** `ReadInt64` and `ReadUint64` in `integer.go`, with expanded tests.

- Accept every integer wire form when its value fits the destination type.
- Check signedness and range before conversion; avoid overflow in intermediate values.
- Test fixtures from task 3 plus manually specified wider encodings for small values.
- For each multi-byte form, test every truncated prefix. Include maximum unsigned
  input read as signed and negative signed input read as unsigned.

**Acceptance:** exact values, consumption counts, trailing-byte preservation, error
classification, and zero consumption on all failures are verified independently of
round-trip tests.

**Progress:** added `ReadInt64` and `ReadUint64` in `integer.go`. Both accept
every integer wire form when representable, including wider encodings of small
values; signedness and range checks precede conversions. Literal table fixtures
cover encoding boundaries, extrema, nonminimal forms, error precedence, unchanged
input, trailing values, and zero failure values/consumption. Every truncated prefix
of all eight multi-byte integer forms is tested, including values that overflow
only once complete. Applied `gofmt`; `go test ./...`, `go vet ./...`, and
`git diff --check` pass with `GOCACHE=/private/tmp/messagepack-go-cache`.
No unresolved issues or later-task APIs were added.

### 5. Encode and decode floating-point values

- [x] Complete task 5.

**Deliverables:** `float.go` and `float_test.go`.

- Implement `AppendFloat32`, `AppendFloat64`, `ReadFloat32`, and `ReadFloat64`.
- Use standard-library bit conversions. Do not compress float64 to float32 or implement
  automatic numeric coercion.
- Test finite values, positive/negative zero, positive/negative infinity, and NaN.
  Use bit comparisons for same-width preservation and `IsNaN` for NaN conversion;
  do not compare NaN with ordinary equality.
- Cover widening, rejected narrowing, integer type mismatches, and all truncated prefixes.

**Acceptance:** independent wire fixtures and same-width bit-preservation tests pass;
widened float32 values follow the documented conversion contract.

**Progress:** added `AppendFloat32`, `AppendFloat64`, `ReadFloat32`, and
`ReadFloat64` in `float.go` using standard-library bit conversions and big-endian
helpers. Independent literal fixtures verify same-width bits for finite values,
subnormals, signed zero, infinities, and quiet/signaling NaN payloads. Tests cover
float32 widening, NaN conversion, rejected narrowing/integer coercion, every
truncated prefix, error precedence, unchanged input, trailing values, preserved
prefixes, and destination capacity reuse. Applied `gofmt`; `go test ./...`,
`go vet ./...`, and `git diff --check` pass with
`GOCACHE=/private/tmp/messagepack-go-cache`. No unresolved issues.

### 6. Encode and decode strings and binary payloads

- [x] Complete task 6.

**Deliverables:** `string.go`, `bytes.go`, and corresponding tests.

- Implement `AppendString`, `AppendBytes`, `ReadString`, `ReadStringBytes`, and `ReadBytes`.
  Append functions return errors for lengths outside the wire range.
- Append string contents directly without an intermediate byte-slice copy. Borrow
  payload slices for the view APIs and use safe conversion for the owned string API.
- Test payload lengths 0, 1, 31, 32, 255, 256, 65535, and 65536 as applicable, including
  embedded NUL, multibyte text, and invalid UTF-8 bytes.
- Verify aliasing by mutating source bytes after reading a view; verify owned strings
  do not change. Verify view capacity equals length and append cannot overwrite the
  following value. Cover nil-as-empty-binary and explicit nil type mismatches.
- Test incomplete length fields, incomplete payloads, and huge advertised lengths with
  tiny inputs. Exercise length-overflow checks with small private-helper tests if needed;
  do not allocate multi-gigabyte fixtures or introduce a public API just for testing.

**Acceptance:** exact bytes, ownership behavior, safe bounds handling, wider valid
headers, append failure atomicity where testable, and trailing values are covered.

**Progress:** added `AppendString`, `AppendBytes`, `ReadString`, `ReadStringBytes`,
and `ReadBytes` with direct string appends, safely owned strings, and borrowed views
whose capacity equals their length. Shared payload bounds checking compares the
advertised length with available input before converting to `int`; no read allocates
from an advertised size alone. Literal header fixtures cover all planned size
boundaries, wider headers, raw bytes, source aliasing, string-copy isolation, trailing
values, prefixes, and buffer reuse. Tests cover every truncated header/payload prefix,
huge advertised lengths, error precedence, and private header overflow without large
fixtures; overflow preserves the destination backing storage. Applied `gofmt`;
`go test ./...`, `go vet ./...`, and `git diff --check` pass with
`GOCACHE=/private/tmp/messagepack-go-cache`. No unresolved issues.

### 7. Encode and decode array and map headers

- [ ] Complete task 7.

**Deliverables:** `array.go`, `map.go`, and corresponding tests.

- Implement `AppendArrayHeader`, `ReadArrayHeader`, `AppendMapHeader`, and `ReadMapHeader`
  using `uint32` counts. Append operations need no count-validation error return.
- Test counts 0, 1, 15, 16, 65535, 65536, and the maximum `uint32` using header-only
  fixtures. Never allocate a collection of the advertised size.
- Test larger-than-necessary headers, truncated headers, and type mismatches.
- Document that header-only success does not prove a complete container exists.

**Acceptance:** header bytes/counts/consumption are correct. Reading a complete header
with no children succeeds; reading the required child from that input fails normally.
No container allocation, recursive traversal, or collection-building API is introduced.

### 8. Demonstrate a complete application message

- [ ] Complete task 8.

**Deliverables:** `example_test.go` and focused integration tests.

- Encode a fixed-schema message containing an ID, name, boolean, float, binary payload,
  and a nested array, using explicit map/array headers. Decode it with the same typed
  API and validate expected field names and counts in the example.
- Keep any example record type in test code. Do not add struct reflection, an application
  model, or general schema machinery to the production package.
- Include independently written expected bytes for a small representative nested message;
  test decoding that fixture as well as encoding it.
- Demonstrate reusing `dst[:0]`, advancing through concatenated messages, copying a
  borrowed payload when ownership is needed, and checking for unwanted trailing bytes.
- Explain that fixed-schema decoding rejects unexpected fields; unknown-field skipping
  is deferred rather than hidden in the example.

**Acceptance:** runnable examples have stable expected output, validate their input,
handle errors explicitly, and show useful end-to-end behavior without adding public APIs.

### 9. Harden decoding with fuzz and portability checks

- [ ] Complete task 9.

**Deliverables:** focused fuzz tests and any missing regression cases.

- Fuzz each read API with arbitrary byte slices. Seed supported tags, the reserved tag,
  extension tags, boundary values, truncations, and extreme advertised lengths.
- Assert no panic, no input mutation, valid consumed-byte bounds on success, and zero
  consumption/zero value on failure. Use explicit calls or small focused fuzz targets;
  avoid reflection-based test harnesses.
- Add semantic round-trip fuzzing for integers, floats, strings, and binary. Do not
  demand identical re-encoded bytes for valid non-minimal inputs. Bound generated
  payload sizes so the harness itself cannot create excessive allocations.
- Run each fuzz target separately for at least 30 seconds and preserve useful failing
  inputs as regression cases. Report targets and durations.
- Cross-compile tests for a 32-bit target to catch assumptions about `int`; run them if
  the environment supports it. Record compilation versus execution accurately.

**Acceptance:** all unit tests pass, each fuzz target completes its recorded smoke run,
and no decoding path allocates based solely on an advertised size. If production fixes
are needed, respect the repository's implementation authorization boundary.

### 10. Measure allocations and establish a performance baseline

- [ ] Complete task 10.

**Deliverables:** `benchmark_test.go`, allocation regression tests, and
`docs/poc-benchmarks.md`.

- Benchmark primitive operations, small/large payloads, headers, and the representative
  message. Separate encoding from decoding and borrowed reads from owned strings.
- Report allocations. Prebuild input and allocate reusable destinations outside timed
  sections; reset slice lengths per iteration. Consume results so work remains observable.
- Measure fresh destinations separately from preallocated/reused destinations. Include
  a payload large enough to exercise ordinary buffer growth in the fresh-buffer case.
- Use `testing.AllocsPerRun` for representative successful primitive reads, borrowed
  payload reads, header reads, and preallocated appends. Assert zero only for these
  explicitly scoped paths; let owned strings escape when measuring their copying cost.
- Record Go version, OS/architecture, hardware, commands, input sizes, and results from
  `go test -run '^$' -bench . -benchmem -count=5 ./...`.
- If a target allocates, inspect the cause and propose the smallest clear correction.
  Do not add pooling, unsafe conversions, caches, specialized code generation, or
  abstractions to chase a score. Record a justified exception if simplicity wins.

**Acceptance:** reproducible results support every allocation claim. No arbitrary
nanosecond threshold, competitor ranking, or blanket zero-allocation claim is required.

### 11. Finish documentation and review the PoC

- [ ] Complete task 11.

**Deliverables:** replacement `README.md`, reviewed GoDoc, and completed task handoff.

- Expand the minimal README with the actual MessagePack PoC scope, a small
  verified usage example, buffer reuse, borrowed-view lifetimes, string-copy behavior,
  error semantics, and links to the contract and benchmark results.
- State extension/timestamp and other deferred features explicitly. Explain that callers
  validate schemas, container contents, and full-message consumption.
- Remove unrelated artwork, links, API examples, and unverified coverage badges. Preserve
  accurate authorship/contribution policy; only change that policy with maintainer authority.
- Review the exported surface for unused convenience APIs and unnecessary abstractions.
  Suggest removals for review instead of performing an unrelated implementation rewrite.
- Run the normal checks, executable examples, and `go test -race ./...` as a final check.
  Race results do not imply concurrent mutation of shared buffers is supported.
- Confirm the PoC can encode/decode the representative message, reject malformed supported
  inputs without panicking, and meet or explain its measured allocation targets.

**Acceptance:** documentation matches implemented behavior, every completed task has
verification evidence, and remaining limitations are explicit. The PoC is complete
without implementing any deferred feature.
