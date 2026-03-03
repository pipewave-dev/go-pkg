package validation

import (
	"context"
	"reflect"
	"time"
)

var (
	// timeDurationType = reflect.TypeOf(time.Duration(0))
	timeType = reflect.TypeOf(time.Time{})
)

func (v *validation) Validate(ctx context.Context, s Validator) (vErrs []ValidationErr) {
	visited := make(map[uintptr]bool)
	return v.validateRecursive(ctx, s, visited)
}

func (v *validation) validateRecursive(ctx context.Context, s Validator, visited map[uintptr]bool) (vErrs []ValidationErr) {
	// Add logic to track visited structs using reflect.Value.Pointer()
	ptr := reflect.ValueOf(s).Pointer()
	if visited[ptr] {
		return
	}
	visited[ptr] = true

	// validate by Validator
	errs := s.Validate(ctx)
	vErrs = append(vErrs, errs...)

	// Recursive check the field has tag `dive`
	// For loop each field in s and validate
	val := reflect.ValueOf(s)

	if val.Kind() == reflect.Ptr && !val.IsNil() {
		val = val.Elem()
	}
	// Stop recursive validating when type is not Struct
	if val.Kind() != reflect.Struct || val.Type().ConvertibleTo(timeType) {
		return
	}

	// do validate on Field
	for i := 0; i < val.NumField(); i++ {
		field := val.Type().Field(i)
		// Skip unexported fields
		if field.PkgPath != "" {
			continue
		}
		// Check field tag `dive`
		if field.Tag.Get("validation") != "dive" {
			continue
		}
		// recursive
		fieldVal := val.Field(i)
		// Ensure the field is addressable
		if !fieldVal.CanAddr() {
			continue
		}

		if valData, ok := fieldVal.Interface().(Validator); ok {
			errs := v.validateRecursive(ctx, valData, visited)
			vErrs = append(vErrs, errs...)
		} else {
			panic("validation dive tag executed on an invalid type of Validator")
		}
	}
	return vErrs
}
