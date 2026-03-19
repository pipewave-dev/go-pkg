package voAuth

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

// === Factory functions

func UserWebsocketAuth(userID string, instanceID string) WebsocketAuth {
	if userID == "" || instanceID == "" {
		panic("voAuth: UserWebsocketAuth called with empty userID or instanceID")
	}
	return WebsocketAuth{
		UserID:     userID,
		InstanceID: instanceID,
	}
}

func AnonymousUserWebsocketAuth(instanceID string) WebsocketAuth {
	if instanceID == "" {
		panic("voAuth: AnonymousUserWebsocketAuth called with empty instanceID")
	}
	return WebsocketAuth{
		InstanceID: instanceID,
	}
}

func UserWebsocketAuthWithMetadata(userID string, instanceID string, metadata map[string]string) WebsocketAuth {
	if userID == "" || instanceID == "" {
		panic("voAuth: UserWebsocketAuthWithMetadata called with empty userID or instanceID")
	}
	return WebsocketAuth{
		UserID:     userID,
		InstanceID: instanceID,
		Metadata:   metadata,
	}
}

func AnonymousUserWebsocketAuthWithMetadata(instanceID string, metadata map[string]string) WebsocketAuth {
	if instanceID == "" {
		panic("voAuth: AnonymousUserWebsocketAuthWithMetadata called with empty instanceID")
	}
	return WebsocketAuth{
		InstanceID: instanceID,
		Metadata:   metadata,
	}
}
