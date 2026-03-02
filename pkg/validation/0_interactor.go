package validation

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

type ValidationProvider interface {
	/*
		# Struct

			- Wrapper of [Validator package].
			- This wrapper add some custom validation tag and some custom error message for tag

		[Validator package]: https://github.com/go-playground/validator
	*/
	Struct(ctx context.Context, s any) (vErrs []ValidationErr)

	Validate(ctx context.Context, s Validator) (vErrs []ValidationErr)

	Var(ctx context.Context, value any, tag string) (errs []ValidationErr)
	/*
		# CreateAlias

			- If you create a new alias with an existing tag before, it will be ignored.
	*/
	CreateAlias(
		ctx context.Context,
		alias string,
		tags string,
		msgFn func(context.Context, validator.FieldError) string,
	)

	/*
		# CreateTagValidation

			- Should use on init() function. This function is not safe for thread, and only run 1 time or PANIC
			- PANIC if you create a new validation with an existing tag before

	*/
	CreateTagValidation(
		ctx context.Context,
		tag string,
		validationFn func(validator.FieldLevel) bool,
		msgFn func(context.Context, validator.FieldError) string,
	)

	// CreateCustomStructValidation.. PANIC if you create a new validation with an existing tag before
	CreateCustomStructValidation(
		fn validator.StructLevelFunc,
		typE interface{},
		mapCusTagMsgFn map[string]func(context.Context, validator.FieldError) string,
	)

	RegisterStructValidationMapRules(rules map[string]string, types ...interface{})

	Map(ctx context.Context, data map[string]interface{}, rules map[string]interface{}) (errs []ValidationErr)
}

type validation struct {
	validate        *validator.Validate
	mappingTagMsgfn map[string]func(context.Context, validator.FieldError) string
}

func NewValidationProvider(
	customTagRegister map[string]func(validator.FieldLevel) bool,
	aliasRegister map[string]string,
	// func should support I18n in context
	customTagMsg map[string]func(context.Context, validator.FieldError) string,
) ValidationProvider {
	v := validator.New()
	ins := &validation{
		v,
		customTagMsg,
	}
	registerCustomGetTagName(ins.validate)

	// CustomTag register
	for tag, fn := range customTagRegister {
		_, ok := customTagMsg[tag]
		if !ok {
			err := fmt.Errorf("Can't create ValidationCustomTag [%s]. Missing msgFn for this tag", tag)
			slog.Error("PANIC", slog.Any("err", err))
			panic(err)
		}
		err := v.RegisterValidation(tag, fn)
		if err != nil {
			err := fmt.Errorf("validation: registerCustomValidations error [%w]", err)
			slog.Error("PANIC", slog.Any("err", err))
			panic(err)
		}
	}
	// Alias register
	for alias, tags := range aliasRegister {
		_, ok := customTagMsg[alias]
		if !ok {
			err := fmt.Errorf("Can't create ValidationAlias [%s] with tags [%s]. Missing msgFn for this alias", alias, tags)
			slog.Error("PANIC", slog.Any("err", err))
			panic(err)
		}
		v.RegisterAlias(alias, tags)
	}
	return ins
}

const (
	TagNameI18n = "i18n"
	TagNameJSON = "json"
	TagNameForm = "form"
	TagNameUri  = "uri"
)

func registerCustomGetTagName(v *validator.Validate) {
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		i18nTag, ok := fld.Tag.Lookup(TagNameI18n)
		if ok {
			return i18nTag
		}
		jsonTag, ok := fld.Tag.Lookup(TagNameJSON)
		if ok {
			name := strings.SplitN(jsonTag, ",", 2)[0]
			return name
		}
		formTag, ok := fld.Tag.Lookup(TagNameForm)
		if ok {
			name := strings.SplitN(formTag, ",", 2)[0]
			return name
		}
		uriTag, ok := fld.Tag.Lookup(TagNameUri)
		if ok {
			name := strings.SplitN(uriTag, ",", 2)[0]
			return name
		}
		return fld.Name
	})
}
