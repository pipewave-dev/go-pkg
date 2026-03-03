package userRepo

import (
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/jackc/pgx/v5/pgxpool"
)

type userRepo struct {
	cfg  configprovider.ConfigStore
	pool *pgxpool.Pool
	obs  observer.Observability
}

func New(
	cfg configprovider.ConfigStore,
	pool *pgxpool.Pool,
	obs observer.Observability,
) repository.User {
	ins := &userRepo{
		cfg:  cfg,
		pool: pool,
		obs:  obs,
	}
	return ins
}
