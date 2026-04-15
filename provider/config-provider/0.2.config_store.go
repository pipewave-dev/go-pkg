package configprovider

import (
	gonanoid "github.com/matoous/go-nanoid/v2"
)

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

func (e *globalEnvT) validate() {
	e.Otel.Validate()

	e.RateLimiter.Validate()
	e.ActiveConnection.Validate()
}

func (e *globalEnvT) loadDefault() {
	{
		e.Version = "v0.1.1"
	}

	if e.ContainerID == "" {
		e.ContainerID = generateUniqueID() // Auto generate an unique ID
	}
}

func generateUniqueID() string {
	nid := gonanoid.Must(12)
	return nid
}
