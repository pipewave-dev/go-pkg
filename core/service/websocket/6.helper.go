package websocket

// Return messagetype.WebSocketReponse wrapped, when receive data, unwrap it first
func WrapperBytesToWebsocketResponse(id, responseToId string, msgType MessageType, data []byte) []byte {
	var res WebsocketResponse
	res.Id = id
	res.ResponseToId = responseToId
	res.MsgType = msgType
	res.Binary = data

	return res.Marshall()
}

// Return messagetype.WebSocketReponse wrapped, when receive data, unwrap it first
func WrapperErrorToWebsocketResponse(id, responseToId string, msgType MessageType, err error) []byte {
	var res WebsocketResponse
	res.Id = id
	res.ResponseToId = responseToId
	res.MsgType = msgType
	res.Error = err.Error()

	return res.Marshall()
}
