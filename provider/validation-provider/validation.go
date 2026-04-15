package validationprovider

import (
	"github.com/pipewave-dev/go-pkg/pkg/validation"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (validation.ValidationProvider, error) {
	validationIns := validation.NewValidationProvider(nil, nil, nil)

	return validationIns, nil
}
