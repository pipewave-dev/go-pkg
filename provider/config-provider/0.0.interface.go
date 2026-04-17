package configprovider

import (
	types "github.com/pipewave-dev/go-pkg/export/types"
	"github.com/pipewave-dev/go-pkg/global/constants"
)

type globalEnvT struct {
	types.EnvType
	Version string
	Fns     *Fns
}

func (g *globalEnvT) LoadDefault() {
	g.EnvType.LoadDefault()
	g.Version = constants.Version
}

func (g *globalEnvT) Validate() {
	g.EnvType.Validate()
}

// ConfigStore provides access to application configuration
type ConfigStore interface {
	// Env returns the global environment configuration
	Env() *globalEnvT

	SetFns(fns *types.Fns)
}
