package timeprovider

import (
	timeprovider "github.com/pipewave-dev/go-pkg/pkg/time-provider"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

func New(cfg configprovider.ConfigStore) timeprovider.TimeProvider {
	env := cfg.Env()
	return timeprovider.New(timeprovider.Config{
		TimeLocation: env.TimeLocation,
	})
}
