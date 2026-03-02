package validation

import (
	"reflect"
)

type ValidationErr struct {
	StructField     string
	StructNamespace string
	Field           string
	TagName         string
	Param           []string
	Value           any
	Kind            reflect.Kind
	Type            reflect.Type
	Message         string // I18n message
}

func (e ValidationErr) Error() string {
	return e.Message
}
