//go:build wireinject
// +build wireinject

package app

import (
	"log/slog"

	"github.com/pipewave-dev/go-pkg/core/repository"
	_ "github.com/pipewave-dev/go-pkg/shared/aerror"

	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/pipewave-dev/go-pkg/provider/queue"
	"github.com/google/wire"
)

func NewPipewave(
	config configprovider.ConfigStore,
	s *slog.Logger,
	rf repository.RepoFactory,
	qf queue.QueueFactory,
) *AppDI {
	wire.Build(IteractorCollection)

	return &AppDI{}
}
