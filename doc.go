// Package messagepack implements typed MessagePack streaming over io.Reader and
// io.Writer, with lower-level slice append/read operations also available.
// It supports nil, booleans, signed/unsigned integers, floats, strings, binary,
// container headers, and bounded skipping. Extensions and timestamps are unsupported.
//
// # HTTP bodies and files
//
// NewDecoder can read directly from an HTTP response body. Callers traverse their
// schema with typed methods and use Decoder.DecodeBytesTo or Decoder.DecodeStringTo
// for large payloads. NewEncoder writes headers and children incrementally to a
// file or other writer; Encoder.EncodeBytesFrom and Encoder.EncodeStringFrom take
// known byte lengths. Transfer scratch and payload requests are at most 32 KiB,
// independent of payload length. Unknown-length input requires an application
// sequence/chunk schema or external counting/staging. Payload lengths and container
// counts cannot exceed uint32 maximum. Array counts are elements; map counts are pairs.
//
// Callers own readers, writers, response bodies, deadlines, cancellation, and byte
// bounds. Codecs never flush, close, read ahead, or buffer output. Caller-supplied
// bufio wrappers must remain in use for subsequent access. Encode success means
// writer acceptance, not durable storage; callers check flush and close separately.
// Codec instances must not be copied after first use or used concurrently.
//
// # Owned results, limits, and schemas
//
// Decoder.DecodeString and Decoder.DecodeBytes return owned results stable across
// subsequent calls and Reset. They allocate proportionally to accepted payloads,
// with a default 16 MiB per-result cap checked before payload allocation or reads.
// Decoder.SetLimits atomically replaces DecoderLimits; zero fields select finite
// defaults. Transfers ignore these owned-result and Skip limits.
//
// Decoder.Skip consumes one complete supported value, including nested containers,
// leaving the next value available. Per-call defaults are 64 container levels,
// 1048576 values, and 64 MiB of wire bytes; depth has a hard ceiling of 256.
// Header methods do not allocate, traverse, or validate children. Callers validate
// schema completeness, counts, field names, duplicates, required fields, and order.
// Strings preserve bytes without UTF-8 validation or normalization. String/binary
// families are distinct; nil binary input encodes empty binary, not nil.
//
// # Failures and message boundaries
//
// Stream EOF before any byte of a requested value matches io.EOF; an incomplete
// operation matches io.ErrUnexpectedEOF. Neither matches the slice-only ErrTruncated.
// EOF before a required child call still appears as io.EOF to a typed decoder;
// the application schema must report that as an incomplete message. Sequences end
// on EOF before the next root. Exactly-one-message checks require a finite body
// or external frame and the same caller buffering; do not wait for connection EOF
// after each message on a persistent stream.
//
// Stream errors may consume a prefix without rollback. Transfer counts include
// destination-accepted payload bytes, even on failure, and exclude headers. Failed
// transfers neither drain nor retry; simultaneous destination failure takes
// precedence over source failure. Ordinary failed results are zero-valued.
// External I/O errors are context-wrapped and preserve errors.Is identity; codec
// sentinels are direct. I/O failures, consumed decode errors, and clean EOF are
// terminal: later operations return the stored error without I/O. Pre-I/O argument
// and encode-length failures leave a healthy attached codec usable. Reset clears
// failure and pending EOF, preserves Decoder limits, and attaches a new stream;
// it cannot repair alignment. Zero values and nil constructor arguments leave
// codecs unattached. Interfaces containing typed nil pointers retain ordinary
// Go method behavior; nil codec receivers are outside the contract.
//
// # Slice operations
//
// Appends preserve the destination prefix; callers retain the returned slice.
// Payload and destination storage must be disjoint. Growth may allocate, while
// a returned append error preserves the destination and its backing storage.
// Slice reads never modify input, leave trailing bytes for the caller, and return
// zero values with zero consumption on failure. Header reads do not validate children.
// ReadBytes and ReadStringBytes borrow input storage with capacity equal to length.
// Source mutation changes these views; copy before independent ownership or source
// reuse. ReadString returns an owned string. Shared mutable buffers require caller
// synchronization. Reflection, automatic struct mapping, and generic object APIs
// remain outside the package's scope.
package messagepack
