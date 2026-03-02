package voAuth

import (
	"errors"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// Auth is a type alias for *auth; nil means unauthenticated (no auth).
type Auth = *auth

type auth struct {
	user   *userAuth
	system *systemAuth
}

func (a *auth) IsSystem() bool {
	return a.system != nil
}

func (a *auth) IsUser() bool {
	return a.user != nil
}

func (a *auth) IsSystemAdmin() bool {
	return a.IsUser() && a.user.isSystemAdmin
}

func (a *auth) IsAnonymous() bool {
	return a.IsUser() && a.user.isAnonymous
}

func (a *auth) UserID() string {
	if a.IsUser() {
		return a.user.UserID
	}
	panic("voAuth: UserID called on system auth")
}

func (a *auth) InstanceID() string {
	if a.IsUser() {
		return a.user.InstanceID
	}
	panic("voAuth: InstanceID called on system auth")
}

func (a *auth) String() string {
	if a.IsUser() {
		return fmt.Sprintf("User: %s@%s", a.user.UserID, a.user.InstanceID)
	}
	return fmt.Sprintf("System: %s", a.system.SystemName)
}

type userAuth struct {
	UserID        string
	InstanceID    string
	isAnonymous   bool
	isSystemAdmin bool
}

type systemAuth struct {
	SystemName string
}

// msgpackAuth is a flat intermediate struct used for msgpack encode/decode.
type msgpackAuth struct {
	SystemName    string `msgpack:"s,omitempty"`
	UserID        string `msgpack:"i,omitempty"`
	InstanceID    string `msgpack:"t,omitempty"`
	IsAnonymous   bool   `msgpack:"a,omitempty"`
	IsSystemAdmin bool   `msgpack:"d,omitempty"`
}

func (d auth) Encode() []byte {
	var data msgpackAuth
	if d.system != nil {
		data.SystemName = d.system.SystemName
	} else if d.user != nil {
		data.UserID = d.user.UserID
		data.InstanceID = d.user.InstanceID
		data.IsAnonymous = d.user.isAnonymous
		data.IsSystemAdmin = d.user.isSystemAdmin
	}
	encoded, err := msgpack.Marshal(&data)
	if err != nil {
		return nil
	}
	return encoded
}

func (d *auth) Decode(b []byte) error {
	if d == nil {
		return errors.New("voAuth: Decode called on nil auth")
	}
	var data msgpackAuth
	if err := msgpack.Unmarshal(b, &data); err != nil {
		return err
	}
	if data.SystemName != "" {
		d.system = &systemAuth{SystemName: data.SystemName}
		d.user = nil
	} else {
		d.user = &userAuth{
			UserID:        data.UserID,
			InstanceID:    data.InstanceID,
			isAnonymous:   data.IsAnonymous,
			isSystemAdmin: data.IsSystemAdmin,
		}
		d.system = nil
	}
	return nil
}

func (t userAuth) MarshalMsgpack() ([]byte, error) {
	data := msgpackAuth{
		UserID:        t.UserID,
		InstanceID:    t.InstanceID,
		IsAnonymous:   t.isAnonymous,
		IsSystemAdmin: t.isSystemAdmin,
	}
	return msgpack.Marshal(&data)
}

func (t *userAuth) UnmarshalMsgpack(b []byte) error {
	var data msgpackAuth
	if err := msgpack.Unmarshal(b, &data); err != nil {
		return err
	}
	t.UserID = data.UserID
	t.InstanceID = data.InstanceID
	t.isAnonymous = data.IsAnonymous
	t.isSystemAdmin = data.IsSystemAdmin
	return nil
}

// === Factory functions

func UserAuth(userID string, instanceID string, isSystemAdmin bool) Auth {
	if userID == "" || instanceID == "" {
		panic("voAuth: UserAuth called with empty userID or instanceID")
	}
	return &auth{
		user: &userAuth{
			UserID:        userID,
			InstanceID:    instanceID,
			isAnonymous:   false,
			isSystemAdmin: isSystemAdmin,
		},
	}
}

func SystemAuth(systemName string) Auth {
	if systemName == "" {
		panic("voAuth: SystemAuth called with empty systemName")
	}
	return &auth{
		system: &systemAuth{SystemName: systemName},
	}
}

func AnonymousUserAuth(instanceID string) Auth {
	if instanceID == "" {
		panic("voAuth: AnonymousUserAuth called with empty instanceID")
	}
	return &auth{
		user: &userAuth{
			InstanceID:  instanceID,
			isAnonymous: true,
		},
	}
}

func NoAuth() Auth {
	// Should not reachable
	return &auth{
		user: &userAuth{
			InstanceID:  "noauth",
			isAnonymous: true,
		},
	}
}

// Decode decodes bytes into an Auth value. Returns (nil, err) if b is empty or invalid.
func Decode(b []byte) (Auth, error) {
	if len(b) == 0 {
		return nil, errors.New("voAuth: empty input")
	}
	a := &auth{}
	if err := a.Decode(b); err != nil {
		return nil, err
	}
	return a, nil
}
