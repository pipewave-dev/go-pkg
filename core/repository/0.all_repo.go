package repository

import (
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/samber/do/v2"
)

type RepoFactory func(
	c configprovider.ConfigStore,
	obs observer.Observability,
) AllRepository

type RepositoryDIFactory = func(i do.Injector) (AllRepository, error)

type AllRepository interface {
	ActiveConnStore() ActiveConnStore
	User() User
	PendingMessage() PendingMessageRepo

	RunMigration() error
}
