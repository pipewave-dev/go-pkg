package validation

import (
	"context"
	"fmt"

	"github.com/go-playground/validator/v10"
)

func (v *validation) Map(ctx context.Context, data map[string]interface{}, rules map[string]interface{}) (errs []ValidationErr) {
	mapErrs := v.validate.ValidateMapCtx(ctx, data, rules)
	if mapErrs == nil {
		return nil
	}

	for fieldName, err := range mapErrs {
		if validationErr, ok := err.(validator.ValidationErrors); ok {
			for _, vErr := range validationErr {
				e := ValidationErr{
					StructField:     fieldName,
					StructNamespace: vErr.StructNamespace(),
					TagName:         vErr.Tag(),
					Field:           fieldName,
					Value:           vErr.Value(),
					Message:         v.getMsgFromFieldError(ctx, vErr),
				}
				errs = append(errs, e)
			}
		} else {
			errs = append(errs, ValidationErr{
				Field:   fieldName,
				Message: fmt.Sprintf("%v", err),
			})
		}
	}

	return errs
}
