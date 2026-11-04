// Package messagepack implements typed MessagePack streaming over io.Reader and
// io.Writer. It supports nil, booleans, signed/unsigned integers, floats, strings,
// binary, container headers, and bounded skipping. Extensions and timestamps are
// unsupported.
//
// Encoder.Encode accepts values implementing Encodee; Decoder.Decode accepts
// targets implementing Decodee. Their EncodeMessagePack and DecodeMessagePack
// methods receive concrete Writer and Reader values. Other values return
// ErrUnsupportedType. Reflection and automatic object mapping are deferred.
// Encoder and Decoder expose only their value operation and Close. Close releases
// the codec without flushing, draining, or closing the supplied stream, returns
// any stored failure, and makes later value operations return ErrClosed.
//
// # HTTP bodies and files
//
// NewReader can read directly from an HTTP response body. Callers traverse their
// schema with typed methods and use Reader.DecodeBytesTo or Reader.DecodeStringTo
// for large payloads. NewWriter writes headers and children incrementally to a
// file or other writer; Writer.EncodeBytesFrom and Writer.EncodeStringFrom take
// known byte lengths. Transfer scratch and payload requests are at most 32 KiB,
// independent of payload length. Unknown-length input requires an application
// sequence/chunk schema or external counting/staging. Payload lengths and container
// counts cannot exceed uint32 maximum. Array counts are elements; map counts are pairs.
//
// Callers own readers, writers, response bodies, deadlines, cancellation, and byte
// bounds. Codecs never flush, close, read ahead, or buffer output. Caller-supplied
// bufio wrappers must remain in use for subsequent access. Encode success means
// writer acceptance, not durable storage; callers check flush and close separately.
// Initialized Writer copies share output, buffering, and failure state. Initialized
// Reader copies share input, EOF, limits, buffering, and failure state. Encoder and
// Decoder instances must not be copied after first use. Shared instances require
// synchronization.
//
// # Owned results, limits, and schemas
//
// Reader.DecodeString and Reader.DecodeBytes return owned results stable across
// subsequent calls, input mutation, and Reset. They allocate proportionally to
// accepted payloads, with a default 16 MiB per-result cap checked before payload
// allocation or reads. Reader.SetLimits atomically replaces ReaderLimits; zero
// fields select finite defaults. Transfers ignore owned-result and Skip limits.
//
// Reader.Skip consumes one complete supported value, including nested containers,
// leaving the next value available. Per-call defaults are 64 container levels,
// 1048576 values, and 64 MiB of wire bytes; depth has a hard ceiling of 256.
// Header methods do not allocate, traverse, or validate children. Callers validate
// schema completeness, counts, field names, duplicates, required fields, and order.
// Strings preserve bytes without UTF-8 validation or normalization. String/binary
// families are distinct; nil binary input encodes empty binary, not nil.
//
// # Failures and message boundaries
//
// EOF before any byte of a requested value matches io.EOF; an incomplete operation
// matches io.ErrUnexpectedEOF. EOF before a required child call still appears as
// io.EOF to a typed Reader; the application schema must report an incomplete
// message. Sequences end on EOF before the next root. Exactly-one-message checks
// require a finite body or external frame and the same caller buffering; do not
// wait for connection EOF after each message on a persistent stream.
//
// Errors may consume a prefix without rollback. Transfer counts include accepted
// payload bytes even on failure and exclude headers. Failed transfers neither
// drain nor retry; simultaneous destination failure takes precedence over source
// failure. Ordinary failed results are zero-valued. External I/O errors are wrapped
// with context and preserve errors.Is identity; codec sentinels are direct.
// I/O failures, consumed decode errors, and clean EOF are terminal. Hook failures
// are terminal too. Decoder preserves returned schema errors and surfaces Reader
// failures even when the hook ignores them. Later operations return the stored
// error without I/O. Pre-I/O argument and encode-length failures leave a healthy
// attached Reader or Writer usable.
//
// Writer.Reset and Reader.Reset clear failure and pending EOF, preserve Reader
// limits, and attach a new stream; they cannot repair alignment. Zero values and
// nil constructor arguments leave codecs unattached. Interfaces containing typed
// nil pointers retain ordinary Go method behavior; nil codec receivers are outside
// the contract.
//
// # In-memory values
//
// Attach a Writer or Encoder to bytes.Buffer for output, and a Reader or Decoder
// to bytes.NewReader for input. Reader and Writer methods own all codec logic;
// the former package-level slice append/read APIs are removed. Encoding input and
// output storage must be disjoint when buffers are reused.
package messagepack
