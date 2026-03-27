package repository

import (
	"github.com/pipewave-dev/go-pkg/pkg/observer"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

type RepoFactory func(
	c configprovider.ConfigStore,
	obs observer.Observability,
) AllRepository

type AllRepository interface {
	ActiveConnStore() ActiveConnStore
	User() User
	PendingMessage() PendingMessageRepo
}
