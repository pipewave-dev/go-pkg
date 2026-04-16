package repository

import (
	"github.com/samber/do/v2"
)

type RepositoryAdapter = func(i do.Injector) (AllRepository, error)

type AllRepository interface {
	ActiveConnStore() ActiveConnStore
	User() User
	PendingMessage() PendingMessageRepo

	RunMigration() error
}
