package validationprovider

import (
	"github.com/pipewave-dev/go-pkg/pkg/validation"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (validation.ValidationProvider, error) {
	validationIns := validation.NewValidationProvider(nil, nil, nil)

	return validationIns, nil
}

// New creates a new validation provider.
// This replaces the singleton pattern in singleton/validation with dependency injection.
// Note: This provider doesn't need config as it uses custom tag registration.
func New() validation.ValidationProvider {
	validationIns := validation.NewValidationProvider(nil, nil, nil)

	return validationIns
}
