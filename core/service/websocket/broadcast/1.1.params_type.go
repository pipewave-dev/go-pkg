package broadcast

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

type SendToUserParams struct {
	UserId  string
	MsgType string
	Payload []byte
}

type SendToSessionParams struct {
	UserId     string
	InstanceId string
	MsgType    string
	Payload    []byte
}

type SendToAnonymousParams struct {
	IsSendAll   bool
	InstanceIds []string
	MsgType     string
	Payload     []byte
}

func (p *SendToUserParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("SendToUserParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *SendToUserParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}

func (p *SendToSessionParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("SendToSessionParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *SendToSessionParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}

func (p *SendToAnonymousParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("SendToAnonymousParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *SendToAnonymousParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}

type DisconnectSessionParams struct {
	UserId     string
	InstanceId string
}

func (p *DisconnectSessionParams) Marshal() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("DisconnectSessionParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *DisconnectSessionParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}

type DisconnectUserParams struct {
	UserId string
}

func (p *DisconnectUserParams) Marshal() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("DisconnectUserParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *DisconnectUserParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}

type SendToUsersParams struct {
	UserIds []string
	MsgType string
	Payload []byte
}

func (p *SendToUsersParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("SendToUsersParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *SendToUsersParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}

type BroadcastParams struct {
	Target  int
	MsgType string
	Payload []byte
}

func (p *BroadcastParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("BroadcastParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *BroadcastParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}
