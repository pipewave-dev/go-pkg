package userRepo

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
)

type userRepo struct {
	cfg  configprovider.ConfigStore
	ddbC *dynamodb.Client
	obs  observer.Observability
}

func New(
	cfg configprovider.ConfigStore,
	ddbC *dynamodb.Client,
	obs observer.Observability,
) repository.User {
	ins := &userRepo{
		cfg:  cfg,
		ddbC: ddbC,
		obs:  obs,
	}
	return ins
}
