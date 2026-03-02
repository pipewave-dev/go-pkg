package aerror

import (
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
)

// ADetailError
// AMultiError

// Encode/decode for msgpack

type MsgpackError struct {
	Code   string `msgpack:"c"`
	Msg    string `msgpack:"m"`
	Detail any    `msgpack:"d"`
}

func (e ASingleError) MarshalMsgpack() ([]byte, error) {
	if e == nil {
		return nil, nil
	}
	x := MsgpackError{
		Code: e.ErrorCode().String(),
		Msg:  e.Error(),
	}
	return msgpack.Marshal(x)
}

func (e ASingleError) UnmarshalMsgpack(data []byte) error {
	return fmt.Errorf("ASingleError is designed to be returned as a response only, not received from the client.")
}

func (e ADetailError) MarshalMsgpack() ([]byte, error) {
	if e == nil {
		return nil, nil
	}
	x := MsgpackError{
		Code:   e.ErrorCode().String(),
		Msg:    e.Error(),
		Detail: e.Detail(),
	}
	return msgpack.Marshal(x)
}

func (e ADetailError) UnmarshalMsgpack(data []byte) error {
	return fmt.Errorf("ADetailError is designed to be returned as a response only, not received from the client.")
}
