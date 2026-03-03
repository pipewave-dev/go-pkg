package activeConnRepo

import (
	"github.com/pipewave-dev/go-pkg/core/repository"

	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type activeConnRepo struct {
	c    configprovider.ConfigStore
	ddbC *dynamodb.Client
	obs  observer.Observability
}

func New(
	c configprovider.ConfigStore,
	ddbC *dynamodb.Client,
	obs observer.Observability,
) repository.ActiveConnStore {
	ins := &activeConnRepo{
		c:    c,
		ddbC: ddbC,
		obs:  obs,
	}
	return ins
}
