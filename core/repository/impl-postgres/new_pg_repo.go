package implpostgres

import (
	"context"

	"github.com/pipewave-dev/go-pkg/core/repository"
	activeConnRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres/active_conn"
	pendingMessageRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres/pending_message"
	userRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres/user"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	"github.com/samber/do/v2"

	"github.com/jackc/pgx/v5/pgxpool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/provider/postgres"
)

func NewDIPostgresRepo(i do.Injector) (repository.AllRepository, error) {
	c := do.MustInvoke[configprovider.ConfigStore](i)
	obs := do.MustInvoke[observer.Observability](i)
	pool := postgres.New(c)
	acs := activeConnRepo.New(c, pool, obs)
	u := userRepo.New(c, pool, obs)
	pm := pendingMessageRepo.New(c, pool, obs)
	return &pgRepo{
		cfg:  c,
		pool: pool,
		acs:  acs,
		u:    u,
		pm:   pm,
	}, nil
}

type pgRepo struct {
	cfg  configprovider.ConfigStore
	pool *pgxpool.Pool
	acs  repository.ActiveConnStore
	u    repository.User
	pm   repository.PendingMessageRepo
}

func (r *pgRepo) ActiveConnStore() repository.ActiveConnStore {
	return r.acs
}

func (r *pgRepo) User() repository.User {
	return r.u
}

func (r *pgRepo) PendingMessage() repository.PendingMessageRepo {
	return r.pm
}

func (r *pgRepo) RunMigration() error {
	return postgres.RunMigration(context.Background(), r.pool)
}
