package validation

import (
	"context"
	"errors"
	"strings"

	"github.com/go-playground/validator/v10"
)

type ValidateOverride interface {
	Validate(ctx context.Context, vlp ValidationProvider) (errs []ValidationErr)
}

func (v *validation) Struct(ctx context.Context, s any) (errs []ValidationErr) {

	err := v.validate.StructCtx(ctx, s)
	if err == nil {
		return nil
	}

	var validateErrs validator.ValidationErrors
	_ = errors.As(err, &validateErrs)
	for _, vErr := range validateErrs {
		// vErr.Param() string
		// vErr.Kind() reflect.Kind
		// vErr.Type() reflect.Type

		e := ValidationErr{
			StructField:     vErr.StructField(),
			StructNamespace: vErr.StructNamespace(),
			TagName:         vErr.Tag(),
			Field:           vErr.Field(),
			Value:           vErr.Value(),
			Param:           strings.Split(vErr.Param(), " "),
			Kind:            vErr.Kind(),
			Type:            vErr.Type(),

			Message: v.getMsgFromFieldError(ctx, vErr),
		}

		errs = append(errs, e)
	}
	return errs
}

func (v *validation) getMsgFromFieldError(ctx context.Context, vErr validator.FieldError) string {

	msgFn, ok := v.mappingTagMsgfn[vErr.Tag()]
	if !ok {
		return vErr.Error()
	}
	return msgFn(ctx, vErr)
}
