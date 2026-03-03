package validation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-playground/validator/v10"
)

func (v *validation) CreateTagValidation(
	ctx context.Context,
	tag string,
	validationFn func(validator.FieldLevel) bool,
	msgFn func(context.Context, validator.FieldError) string,
) {
	_, ok := v.mappingTagMsgfn[tag]
	if ok {
		err := fmt.Errorf("can't create exist Validation [%s]", tag)
		slog.Error("PANIC", slog.Any("err", err))
		panic(err)
	}
	err := v.validate.RegisterValidation(tag, validationFn)
	if err != nil {
		err := fmt.Errorf("(*validation).CreateValidation error [%s]", err.Error())
		slog.Error("PANIC", slog.Any("err", err))
		panic(err)
	}
	v.mappingTagMsgfn[tag] = msgFn
}
