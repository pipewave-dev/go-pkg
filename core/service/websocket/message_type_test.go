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
