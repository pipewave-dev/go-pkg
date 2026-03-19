package broadcast

// Channel list with payload type (in comment)
const (
	// SendToUserParams
	channelSendToUser pubsubChannel = "SendToUser"
	// SendToSessionParams
	channelSendToSession pubsubChannel = "SendToSession"
	// SendToAnonymousParams
	channelSendToAnonymous pubsubChannel = "SendToAnonymous"
	// DisconnectSessionParams
	channelDisconnectSession pubsubChannel = "DisconnectSession"
	// DisconnectUserParams
	channelDisconnectUser pubsubChannel = "DisconnectUser"
	// SendToUsersParams
	channelSendToUsers pubsubChannel = "SendToUsers"
	// BroadcastParams
	channelBroadcast pubsubChannel = "Broadcast"
)
