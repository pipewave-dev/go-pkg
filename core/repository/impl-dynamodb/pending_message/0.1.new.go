package pendingMessageRepo

import (
	"fmt"

	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/pkg/dynamodb"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

type pendingMessageRepo struct {
	c   configprovider.ConfigStore
	ddb dynamodb.DynamodbProvider
	obs observer.Observability
}

func New(
	c configprovider.ConfigStore,
	ddb dynamodb.DynamodbProvider,
	obs observer.Observability,
) repository.PendingMessageRepo {
	return &pendingMessageRepo{
		c:   c,
		ddb: ddb,
		obs: obs,
	}
}

func sessionKey(userID, instanceID string) string {
	return fmt.Sprintf("%s:%s", userID, instanceID)
}

type ddbPendingMessage struct {
	SessionKey string // PK: userID:instanceID
	SendAt     int64  // SK: Unix nano
	Message    []byte
	TTL        int64 // UnixMili seconds for DynamoDB TTL
}
