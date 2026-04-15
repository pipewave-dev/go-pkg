package activeConnRepo

import (
	"github.com/pipewave-dev/go-pkg/core/repository"

	"github.com/pipewave-dev/go-pkg/pkg/dynamodb"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

type activeConnRepo struct {
	c   configprovider.ConfigStore
	ddb dynamodb.DynamodbProvider
	obs observer.Observability
}

func New(
	c configprovider.ConfigStore,
	ddb dynamodb.DynamodbProvider,
	obs observer.Observability,
) repository.ActiveConnStore {
	ins := &activeConnRepo{
		c:   c,
		ddb: ddb,
		obs: obs,
	}
	return ins
}
