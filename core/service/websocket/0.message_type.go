package websocket

import (
	"github.com/pipewave-dev/go-pkg/core/service/websocket/wire"
)

// MessageType identifies a message. Control types use a sentinel string that
// cannot be produced by a user-defined type, because control frames are
// encoded out-of-band (msgTypeLen == 0) rather than as message-type bytes.
type MessageType string

const (
	MessageTypeHeartbeat = MessageType("\x00hb")
	MessageTypeAck       = MessageType("\x00ack")
)

// controlCodeFor maps a control MessageType to its on-wire code.
func controlCodeFor(mt MessageType) (byte, bool) {
	switch mt {
	case MessageTypeHeartbeat:
		return wire.ControlHeartbeat, true
	case MessageTypeAck:
		return wire.ControlAck, true
	default:
		return 0, false
	}
}

// messageTypeFor maps an on-wire control code back to its MessageType.
func messageTypeFor(code byte) MessageType {
	switch code {
	case wire.ControlHeartbeat:
		return MessageTypeHeartbeat
	case wire.ControlAck:
		return MessageTypeAck
	default:
		return MessageType("")
	}
}

func toFrame(msgType MessageType, id, responseToId, errStr, ackId string, binary []byte) *wire.Frame {
	f := &wire.Frame{
		ID:           id,
		ResponseToID: responseToId,
		Error:        errStr,
		AckID:        ackId,
		Binary:       binary,
	}
	if code, ok := controlCodeFor(msgType); ok {
		f.ControlCode = code // MsgType stays empty => control frame
	} else {
		f.MsgType = string(msgType)
	}
	return f
}

func msgTypeFromFrame(f *wire.Frame) MessageType {
	if f.MsgType == "" {
		return messageTypeFor(f.ControlCode)
	}
	return MessageType(f.MsgType)
}

type WebsocketResponse struct {
	Id           string
	ResponseToId string
	MsgType      MessageType
	Error        string
	Binary       []byte
	AckId        string
}

// Marshall returns the encoded frame, or nil if a field exceeds its wire
// limit. This mirrors the previous msgpack behaviour of swallowing the error.
func (wsRes *WebsocketResponse) Marshall() []byte {
	data, err := wire.Encode(toFrame(
		wsRes.MsgType, wsRes.Id, wsRes.ResponseToId, wsRes.Error, wsRes.AckId, wsRes.Binary,
	))
	if err != nil {
		return nil
	}
	return data
}

// Unmarshall decodes data into wsRes. The resulting Binary field aliases
// data — it is not a copy. Callers must not mutate data after calling
// Unmarshall, and must not retain wsRes.Binary beyond the lifetime of the
// buffer that data points into.
//
// This currently holds safely because the WebSocket server allocates a
// fresh payload buffer per frame (see server/gobwas/1_server.go), so the
// alias merely keeps that buffer alive via GC. If a pooled or reused read
// buffer is ever introduced there, this aliasing becomes unsafe for any
// caller that retains Binary past the read — notably the async forward
// path in server/fns, which hands Binary to a channel drained by a
// separate goroutine.
func (wsRes *WebsocketResponse) Unmarshall(data []byte) error {
	f, err := wire.Decode(data)
	if err != nil {
		return err
	}
	wsRes.Id = f.ID
	wsRes.ResponseToId = f.ResponseToID
	wsRes.MsgType = msgTypeFromFrame(f)
	wsRes.Error = f.Error
	wsRes.Binary = f.Binary
	wsRes.AckId = f.AckID
	return nil
}

type WebsocketResquest struct {
	Id      string
	MsgType MessageType
	Binary  []byte
}

func (wsReq *WebsocketResquest) Marshall() []byte {
	data, err := wire.Encode(toFrame(wsReq.MsgType, wsReq.Id, "", "", "", wsReq.Binary))
	if err != nil {
		return nil
	}
	return data
}

// Unmarshall decodes data into wsReq. The resulting Binary field aliases
// data — it is not a copy. Callers must not mutate data after calling
// Unmarshall, and must not retain wsReq.Binary beyond the lifetime of the
// buffer that data points into.
//
// This currently holds safely because the WebSocket server allocates a
// fresh payload buffer per frame (see server/gobwas/1_server.go), so the
// alias merely keeps that buffer alive via GC. If a pooled or reused read
// buffer is ever introduced there, this aliasing becomes unsafe for any
// caller that retains Binary past the read — notably the async forward
// path in server/fns, which hands Binary to a channel drained by a
// separate goroutine.
func (wsReq *WebsocketResquest) Unmarshall(data []byte) error {
	f, err := wire.Decode(data)
	if err != nil {
		return err
	}
	wsReq.Id = f.ID
	wsReq.MsgType = msgTypeFromFrame(f)
	wsReq.Binary = f.Binary
	return nil
}
