package implpostgres

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	activeConnRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres/active_conn"
	userRepo "github.com/pipewave-dev/go-pkg/core/repository/impl-postgres/user"
	"github.com/pipewave-dev/go-pkg/pkg/observer"

	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/provider/postgres"
)

func NewPostgresRepo(
	c configprovider.ConfigStore,
	obs observer.Observability,
) repository.AllRepository {
	pool := postgres.New(c)
	acs := activeConnRepo.New(c, pool, obs)
	u := userRepo.New(c, pool, obs)
	return &pgRepo{
		acs: acs,
		u:   u,
	}
}

type pgRepo struct {
	acs repository.ActiveConnStore
	u   repository.User
}

func (r *pgRepo) ActiveConnStore() repository.ActiveConnStore {
	return r.acs
}

func (r *pgRepo) User() repository.User {
	return r.u
}

func (r *pgRepo) PendingMessage() repository.PendingMessageRepo {
	// TODO
	panic("PendingMessageRepo not implemented — add Postgres implementation")
}
