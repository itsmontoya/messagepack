// Package messagepack implements a typed subset of MessagePack using caller-owned
// append buffers and explicit reads. Extensions and timestamps are unsupported.
//
// Appends preserve the destination prefix; callers retain the returned slice.
// Payload and destination storage must be disjoint. Growth may allocate, while
// a returned append error preserves the destination and its backing storage.
//
// Reads never modify input. They consume one value and leave trailing bytes to
// the caller; failures return a zero value and consume nothing. Callers validate
// schemas, traverse containers, and check full-message consumption. Header reads
// do not validate children.
//
// ReadBytes and ReadStringBytes borrow input storage with capacity equal to length.
// Source mutation changes these views; copy them before independent ownership or
// source reuse. ReadString returns an owned string. String APIs preserve bytes
// without UTF-8 validation or normalization. Shared mutable buffers require caller
// synchronization.
package messagepack
