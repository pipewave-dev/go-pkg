package configprovider

import "time"

// configStore is the concrete implementation of ConfigStore interface
type configStore struct {
	env *globalEnvT
}

// Env returns the global environment configuration
func (c *configStore) Env() *globalEnvT {
	return c.env
}

// SetFns sets the function store
func (c *configStore) SetFns(fns *Fns) {
	if fns == nil {
		panic("fns must not be nil")
	}
	if c.env.Fns != nil {
		panic("fns already set")
	}
	c.env.Fns = fns
}

// validate checks if the configuration is valid and panics if any violation is found
func (e *globalEnvT) validate() {
	// Validate Otel configuration
	e.Otel.Validate()

	// Validate RateLimiter configuration
	e.RateLimiter.Validate()
}

// loadDefault sets default values for configuration fields if they are not provided
func (e *globalEnvT) loadDefault() {
	// Set default timezone
	{
		e.Version = "v0.1.1"
	}

	if e.HeartbeatCutoff == 0 {
		e.HeartbeatCutoff = 2 * time.Minute
	}
}
