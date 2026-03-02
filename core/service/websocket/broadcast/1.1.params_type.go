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
