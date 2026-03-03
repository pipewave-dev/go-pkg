package impldynamodb

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	activeConnRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn"
	userRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/user"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	_ "github.com/pipewave-dev/go-pkg/shared/aerror"

	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/provider/dynamodb"
)

func NewDynamoRepo(
	c configprovider.ConfigStore,
	obs observer.Observability,
) repository.AllRepository {
	ddbP := dynamodb.New(c)
	ddbC := ddbP.Client()
	acs := activeConnRepo.New(c, ddbC, obs)
	u := userRepo.New(c, ddbC, obs)
	return &ddbRepo{
		acs: acs,
		u:   u,
	}
}

type ddbRepo struct {
	acs repository.ActiveConnStore
	u   repository.User
}

func (r *ddbRepo) ActiveConnStore() repository.ActiveConnStore {
	return r.acs
}

func (r *ddbRepo) User() repository.User {
	return r.u
}
