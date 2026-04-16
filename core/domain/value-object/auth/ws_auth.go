package voAuth

import "github.com/pipewave-dev/go-pkg/sdk/types"

type WebsocketAuth = types.WebsocketAuth

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
