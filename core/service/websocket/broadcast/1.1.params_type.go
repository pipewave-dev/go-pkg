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

type DisconnectSessionParams struct {
	UserId     string
	InstanceId string
}

type DisconnectUserParams struct {
	UserId string
}

type SendToUsersParams struct {
	UserIds []string
	MsgType string
	Payload []byte
}

type SendToAnonymousParams struct {
	IsSendAll   bool
	InstanceIds []string

	MsgType string
	Payload []byte
}

type SendToAuthenticatedParams struct {
	MsgType string
	Payload []byte
}

type SendToAllParams struct {
	MsgType string
	Payload []byte
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

func (p *SendToAuthenticatedParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("SendToAuthenticatedParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *SendToAuthenticatedParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}

func (p *SendToAllParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("SendToAllParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *SendToAllParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
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

func (p *DisconnectUserParams) Marshal() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("DisconnectUserParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *DisconnectUserParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
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

type SendToSessionWithAckParams struct {
	UserId            string
	InstanceId        string
	MsgType           string
	Payload           []byte
	AckID             string
	SourceContainerID string
}

type SendToUserWithAckParams struct {
	UserId            string
	MsgType           string
	Payload           []byte
	AckID             string
	SourceContainerID string
}

type AckResolvedParams struct {
	AckID string
}

func (p *SendToSessionWithAckParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("SendToSessionWithAckParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *SendToSessionWithAckParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}

func (p *SendToUserWithAckParams) Marshal() ([]byte, error) {
	if p == nil || p.Payload == nil {
		return nil, fmt.Errorf("SendToUserWithAckParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *SendToUserWithAckParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}

func (p *AckResolvedParams) Marshal() ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("AckResolvedParams.Marshal: invalid input")
	}
	return msgpack.Marshal(p)
}

func (p *AckResolvedParams) Unmarshal(b []byte) error {
	return msgpack.Unmarshal(b, p)
}
