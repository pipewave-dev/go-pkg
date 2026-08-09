// Package wire implements the Pipewave WebSocket v1 binary envelope.
// The format is specified in docs/wire-protocol-v1.md; the conformance
// vectors in docs/wire-vectors-v1.json are the cross-language contract.
package wire

import "errors"

const (
	Version byte = 0x1

	ControlHeartbeat byte = 0xCA
	ControlAck       byte = 0xCB

	FlagID           byte = 0x10
	FlagResponseToID byte = 0x20
	FlagAckID        byte = 0x40
	FlagError        byte = 0x80

	MaxShortField = 255
	MaxErrorLen   = 65535
)

var (
	ErrFieldTooLong = errors.New("wire: field exceeds maximum length")
	ErrMalformed    = errors.New("wire: malformed frame")
	ErrVersion      = errors.New("wire: unsupported version")
)

// Frame is the decoded envelope. A Frame with an empty MsgType is a control
// frame identified by ControlCode; otherwise ControlCode is ignored.
// Binary is opaque and is never copied on decode.
type Frame struct {
	MsgType      string
	ControlCode  byte
	ID           string
	ResponseToID string
	AckID        string
	Error        string
	Binary       []byte
}
