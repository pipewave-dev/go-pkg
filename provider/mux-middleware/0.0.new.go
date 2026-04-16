package muxmiddleware

import (
	mm "github.com/pipewave-dev/go-pkg/pkg/mux-middleware"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (mm.MiddlewareProvider, error) {
	cfg := do.MustInvoke[configprovider.ConfigStore](i)
	config := cfg.Env()
	ins := mm.NewMiddlewareProvider(
		&mm.MWConfig{
			IgnoreAccessLogPath: nil,
			TraceIDHeader:       config.ExtractHeader.TraceIDHeader,
		},
	)

	return ins, nil
}
