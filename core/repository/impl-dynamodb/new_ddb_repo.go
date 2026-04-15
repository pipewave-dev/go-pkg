package impldynamodb

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/repository"
	activeConnRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/active_conn"
	pendingMessageRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/pending_message"
	userRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-dynamodb/user"
	pkgdynamodb "github.com/pipewave-dev/go-pkg/pkg/dynamodb"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/samber/do/v2"

	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	dynamodbprovider "github.com/pipewave-dev/go-pkg/provider/dynamodb"
)

func NewDIDynamoDBRepo(i do.Injector) (repository.AllRepository, error) {
	c := do.MustInvoke[configprovider.ConfigStore](i)
	obs := do.MustInvoke[observer.Observability](i)
	ddbP := dynamodbprovider.New(c)
	acs := activeConnRepo.New(c, ddbP, obs)
	u := userRepo.New(c, ddbP, obs)
	pm := pendingMessageRepo.New(c, ddbP, obs)
	return &ddbRepo{
		cfg:  c,
		ddbP: ddbP,
		acs:  acs,
		u:    u,
		pm:   pm,
	}, nil
}

func NewDynamoRepo(
	c configprovider.ConfigStore,
	obs observer.Observability,
) repository.AllRepository {
	ddbP := dynamodbprovider.New(c)
	acs := activeConnRepo.New(c, ddbP, obs)
	u := userRepo.New(c, ddbP, obs)
	pm := pendingMessageRepo.New(c, ddbP, obs)
	return &ddbRepo{
		cfg:  c,
		ddbP: ddbP,
		acs:  acs,
		u:    u,
		pm:   pm,
	}
}

type ddbRepo struct {
	cfg  configprovider.ConfigStore
	ddbP pkgdynamodb.DynamodbProvider
	acs  repository.ActiveConnStore
	u    repository.User
	pm   repository.PendingMessageRepo
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

func (r *ddbRepo) RunMigration() error {
	return dynamodbprovider.RunMigration(context.Background(), r.cfg, r.ddbP)
}
