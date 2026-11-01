// Package messagepack implements a typed subset of MessagePack using caller-owned
// append buffers and explicit reads. Extensions and timestamps are unsupported.
//
// Reads consume one value and leave trailing bytes to the caller. Borrowed binary
// and string views retain their input storage; owned string reads copy the bytes.
// Callers validate schemas, traverse containers, and check full-message consumption.
package messagepack
