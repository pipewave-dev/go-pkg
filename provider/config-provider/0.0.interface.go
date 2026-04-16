package configprovider

import types "github.com/pipewave-dev/go-pkg/sdk/types"

type globalEnvT struct {
	types.EnvType
	Version string
	Fns     *Fns
}

func (g *globalEnvT) LoadDefault() {
	g.EnvType.LoadDefault()
	g.Version = "v0.1.0"
}

// ConfigStore provides access to application configuration
type ConfigStore interface {
	// Env returns the global environment configuration
	Env() *globalEnvT

	SetFns(fns *types.Fns)
}
