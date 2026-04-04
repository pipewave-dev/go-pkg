package impldynamodb

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	activeConnRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn"
	pendingMessageRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/pending_message"
	userRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/user"
	"github.com/pipewave-dev/go-pkg/pkg/observer"

	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/provider/dynamodb"
)

func NewDynamoRepo(
	c configprovider.ConfigStore,
	obs observer.Observability,
) repository.AllRepository {
	ddbP := dynamodb.New(c)
	acs := activeConnRepo.New(c, ddbP, obs)
	u := userRepo.New(c, ddbP, obs)
	pm := pendingMessageRepo.New(c, ddbP, obs)
	return &ddbRepo{
		acs: acs,
		u:   u,
		pm:  pm,
	}
}

type ddbRepo struct {
	acs repository.ActiveConnStore
	u   repository.User
	pm  repository.PendingMessageRepo
}

func (r *ddbRepo) ActiveConnStore() repository.ActiveConnStore {
	return r.acs
}

func (r *ddbRepo) User() repository.User {
	return r.u
}

func (r *ddbRepo) PendingMessage() repository.PendingMessageRepo {
	return r.pm
}
