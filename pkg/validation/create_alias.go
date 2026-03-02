package validation

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/go-playground/validator/v10"
)

func (v *validation) CreateAlias(
	ctx context.Context,
	alias string,
	tags string,
	msgFn func(context.Context, validator.FieldError) string,
) {
	_, ok := v.mappingTagMsgfn[alias]
	if ok {
		slog.Warn(fmt.Sprintf("can't create exist ValidationAlias [%s] with tags [%s]", alias, tags))
		return
	}
	v.validate.RegisterAlias(alias, tags)
	v.mappingTagMsgfn[alias] = msgFn
}
