package validation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-playground/validator/v10"
)

func (v *validation) CreateCustomStructValidation(
	fn validator.StructLevelFunc,
	typE interface{},
	mapCusTagMsgFn map[string]func(context.Context, validator.FieldError) string,
) {
	for cusTag, msgFn := range mapCusTagMsgFn {
		_, ok := v.mappingTagMsgfn[cusTag]
		if ok {
			err := fmt.Errorf("can't create exist Validation [%s]", cusTag)
			slog.Error("PANIC", slog.Any("err", err))
			panic(err)
		}
		v.mappingTagMsgfn[cusTag] = msgFn
	}
	v.validate.RegisterStructValidation(fn, typE)
}
