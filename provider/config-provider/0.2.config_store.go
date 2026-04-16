package configprovider

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
