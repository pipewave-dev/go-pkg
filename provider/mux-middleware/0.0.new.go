package muxmiddleware

import (
	mm "github.com/pipewave-dev/go-pkg/pkg/mux-middleware"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

// New creates a new mux-middleware provider.
func New(
	cfg configprovider.ConfigStore,
) mm.MiddlewareProvider {
	config := cfg.Env()
	ins := mm.NewMiddlewareProvider(
		&mm.MWConfig{
			IgnoreAccessLogPath: nil,
			TraceIDHeader:       config.TraceIDHeader,
		},
	)

	return ins
}
