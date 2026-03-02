package configprovider

import (
	"log"
	"os"
	"time"

	"github.com/samber/lo"
)

// configStore is the concrete implementation of ConfigStore interface
type configStore struct {
	env *globalEnvT
}

// Env returns the global environment configuration
func (c *configStore) Env() *globalEnvT {
	return c.env
}

// validate checks if the configuration is valid and panics if any violation is found
func (e *globalEnvT) validate() {
	// Validate Otel configuration
	e.Otel.Validate()

	// Validate that Fns is not nil
	e.Fns.Validate()

	// Validate RateLimiter configuration
	e.RateLimiter.Validate()
}

// loadDefault sets default values for configuration fields if they are not provided
func (e *globalEnvT) loadDefault() {
	// Set default timezone
	{
		if e.TimezoneStr == nil {
			e.TimezoneStr = lo.ToPtr(os.Getenv("TZ"))
		}
		loc, err := time.LoadLocation(*e.TimezoneStr)
		if err != nil {
			log.Panicf("Parsing failed for TimezoneStr with value [%s]", *e.TimezoneStr)
		}
		e.TimeLocation = loc
		time.Local = loc // Set the global time.Local to the parsed location
	}

	// Load default functions
	if e.Fns != nil {
		e.Fns.LoadDefault()
	}
}
