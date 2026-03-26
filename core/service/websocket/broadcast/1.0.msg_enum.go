package broadcast

// Targeted
const (
	// SendToUserParams
	msgTSendToUser msgType = "SendToUser"
	// SendToSessionParams
	msgTSendToSession msgType = "SendToSession"
	// DisconnectSessionParams
	msgTDisconnectSession msgType = "DisconnectSession"
	// DisconnectUserParams
	msgTDisconnectUser msgType = "DisconnectUser"
	// SendToUsersParams
	msgTSendToUsers msgType = "SendToUsers"
)

// All
const (
	// SendToAnonymousParams
	msgTSendToAnonymous msgType = "SendToAnonymous"
	// SendToAuthenticatedParams
	msgTSendToAuthenticated msgType = "SendToAuthenticated"
	// SendToAllParams
	msgTSendToAll msgType = "SendToAll"
)

const (
	channelPrefix    = "wb"
	broadcastChannel = "wbbc"
)
