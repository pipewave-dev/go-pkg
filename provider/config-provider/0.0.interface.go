package configprovider

// ConfigStore provides access to application configuration
type ConfigStore interface {
	// Env returns the global environment configuration
	Env() *globalEnvT
	SetFns(fns *Fns)
}
