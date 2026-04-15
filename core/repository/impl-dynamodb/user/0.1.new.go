package userRepo

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/pkg/dynamodb"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

type userRepo struct {
	cfg configprovider.ConfigStore
	ddb dynamodb.DynamodbProvider
	obs observer.Observability
}

func New(
	cfg configprovider.ConfigStore,
	ddb dynamodb.DynamodbProvider,
	obs observer.Observability,
) repository.User {
	ins := &userRepo{
		cfg: cfg,
		ddb: ddb,
		obs: obs,
	}
	return ins
}
