package activeConnRepo

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/jackc/pgx/v5/pgxpool"
)

type activeConnRepo struct {
	c    configprovider.ConfigStore
	pool *pgxpool.Pool
	obs  observer.Observability
}

func New(
	c configprovider.ConfigStore,
	pool *pgxpool.Pool,
	obs observer.Observability,
) repository.ActiveConnStore {
	ins := &activeConnRepo{
		c:    c,
		pool: pool,
		obs:  obs,
	}
	return ins
}
