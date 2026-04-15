package pendingMessageRepo

import (
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pipewave-dev/go-pkg/core/repository"
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

type pendingMessageRepo struct {
	c    configprovider.ConfigStore
	pool *pgxpool.Pool
	obs  observer.Observability
}

func New(
	c configprovider.ConfigStore,
	pool *pgxpool.Pool,
	obs observer.Observability,
) repository.PendingMessageRepo {
	return &pendingMessageRepo{
		c:    c,
		pool: pool,
		obs:  obs,
	}
}

func sessionKey(userID, instanceID string) string {
	return fmt.Sprintf("%s:%s", userID, instanceID)
}
