package websocket

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponseRoundTrip(t *testing.T) {
	orig := WebsocketResponse{
		Id:           "id-1",
		ResponseToId: "id-0",
		MsgType:      MessageType("chat.message"),
		Error:        "",
		Binary:       []byte("hello"),
		AckId:        "ack-1",
	}
	data := orig.Marshall()
	require.NotEmpty(t, data)

	var got WebsocketResponse
	require.NoError(t, got.Unmarshall(data))
	require.Equal(t, orig.Id, got.Id)
	require.Equal(t, orig.ResponseToId, got.ResponseToId)
	require.Equal(t, orig.MsgType, got.MsgType)
	require.Equal(t, orig.Binary, got.Binary)
	require.Equal(t, orig.AckId, got.AckId)
}

func TestRequestRoundTrip(t *testing.T) {
	orig := WebsocketResquest{
		Id:      "req-1",
		MsgType: MessageType("chat.send"),
		Binary:  []byte("payload"),
	}
	data := orig.Marshall()
	var got WebsocketResquest
	require.NoError(t, got.Unmarshall(data))
	require.Equal(t, orig.Id, got.Id)
	require.Equal(t, orig.MsgType, got.MsgType)
	require.Equal(t, orig.Binary, got.Binary)
}

// Control types must survive a round trip and must not collide with a
// user-defined MsgType that happens to contain the same bytes.
func TestControlTypesRoundTrip(t *testing.T) {
	for _, mt := range []MessageType{MessageTypeHeartbeat, MessageTypeAck} {
		res := WebsocketResponse{MsgType: mt}
		var got WebsocketResponse
		require.NoError(t, got.Unmarshall(res.Marshall()))
		require.Equal(t, mt, got.MsgType)
	}
}

func TestUserMsgTypeCannotSpoofControl(t *testing.T) {
	spoof := WebsocketResquest{MsgType: MessageType("\xCA")}
	var got WebsocketResquest
	require.NoError(t, got.Unmarshall(spoof.Marshall()))
	require.NotEqual(t, MessageTypeHeartbeat, got.MsgType,
		"a user message type must never decode as the heartbeat control frame")
}

func TestUnmarshallRejectsGarbage(t *testing.T) {
	var got WebsocketResquest
	require.Error(t, got.Unmarshall([]byte{0xFF, 0xFF, 0xFF}))
}

// TestMessageTypeError_IsNotAControlFrame pins that the server's protocol
// error response (MessageTypeError) always has a non-empty MsgType, so
// toFrame encodes it as a normal msgType frame (msgTypeLen != 0), never as
// an (undefined) control frame — see toFrame: an empty MsgType is what
// triggers the msgTypeLen == 0 / control-frame path.
func TestMessageTypeError_IsNotAControlFrame(t *testing.T) {
	res := WebsocketResponse{MsgType: MessageTypeError, Error: "bad frame"}
	data := res.Marshall()
	require.NotEmpty(t, data)
	require.NotZero(t, data[1], "msgTypeLen must be nonzero: MessageTypeError must not encode as a control frame")

	var got WebsocketResponse
	require.NoError(t, got.Unmarshall(data))
	require.Equal(t, MessageTypeError, got.MsgType)
	require.Equal(t, "bad frame", got.Error)
}

// TestMarshall_ReturnsNilOnOverlongField pins the documented Marshall
// contract (an over-long field yields nil, not a truncated/corrupt frame)
// that handleMessage's nil-check depends on to avoid sending a zero-length
// frame to the client.
func TestMarshall_ReturnsNilOnOverlongField(t *testing.T) {
	res := WebsocketResponse{
		Id: string(make([]byte, 256)), // exceeds the 255-byte short-field limit
	}
	require.Nil(t, res.Marshall())
}
