package types

import (
	"errors"

	"github.com/vmihailenco/msgpack/v5"
)

type WebsocketAuth struct {
	UserID     string
	InstanceID string
	Metadata   map[string]string
}

func (ws *WebsocketAuth) IsAnonymous() bool {
	return ws.UserID == ""
}

// encode/decode

func (d WebsocketAuth) Encode() []byte {
	encoded, err := msgpack.Marshal(d)
	if err != nil {
		return nil
	}
	return encoded
}

func (d *WebsocketAuth) Decode(b []byte) error {
	if d == nil {
		return errors.New("voAuth: Decode called on nil auth")
	}
	x := WebsocketAuth{}
	if err := msgpack.Unmarshal(b, &x); err != nil {
		return err
	}
	*d = x
	return nil
}
