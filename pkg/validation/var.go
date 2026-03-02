package validation

import (
	"context"
	"errors"

	"github.com/go-playground/validator/v10"
)

func (v *validation) Var(ctx context.Context, value any, tag string) (errs []ValidationErr) {
	err := v.validate.VarCtx(ctx, value, tag)
	if err == nil {
		return nil
	}

	var validateErrs validator.ValidationErrors
	_ = errors.As(err, &validateErrs)
	for _, vErr := range validateErrs {
		e := ValidationErr{
			StructField:     vErr.StructField(),
			StructNamespace: vErr.StructNamespace(),
			TagName:         vErr.Tag(),
			Field:           vErr.Field(),
			Value:           vErr.Value(),
			Message:         v.getMsgFromFieldError(ctx, vErr),
		}

		errs = append(errs, e)
	}
	return errs
}
