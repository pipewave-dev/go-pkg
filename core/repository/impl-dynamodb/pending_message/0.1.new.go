package pendingMessageRepo

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

type pendingMessageRepo struct {
	c    configprovider.ConfigStore
	ddbC *dynamodb.Client
	obs  observer.Observability
}

func New(
	c configprovider.ConfigStore,
	ddbC *dynamodb.Client,
	obs observer.Observability,
) repository.PendingMessageRepo {
	return &pendingMessageRepo{
		c:    c,
		ddbC: ddbC,
		obs:  obs,
	}
}

func sessionKey(userID, instanceID string) string {
	return fmt.Sprintf("%s:%s", userID, instanceID)
}
