package websocket

import (
	"github.com/vmihailenco/msgpack/v5"
)

type MessageType string

var MessageTypeHeartbeat = MessageType([]byte{202}) // heartbeat

type WebsocketResponse struct {
	Id           string      `msgpack:"i,omitempty"`
	ResponseToId string      `msgpack:"r,omitempty"`
	MsgType      MessageType `msgpack:"t"`
	Error        string      `msgpack:"e,omitempty"`
	Binary       []byte      `msgpack:"b,omitempty"`
}

func (wsRes *WebsocketResponse) Marshall() []byte {
	data, _ := msgpack.Marshal(wsRes)
	return data
}

func (wsRes *WebsocketResponse) Unmarshall(data []byte) error {
	return msgpack.Unmarshal(data, wsRes)
}

type WebsocketResquest struct {
	Id      string      `msgpack:"i,omitempty"`
	MsgType MessageType `msgpack:"t"`
	Binary  []byte      `msgpack:"b,omitempty"`
}

func (wsReq *WebsocketResquest) Marshall() []byte {
	data, _ := msgpack.Marshal(wsReq)
	return data
}

func (wsReq *WebsocketResquest) Unmarshall(data []byte) error {
	return msgpack.Unmarshal(data, wsReq)
}
