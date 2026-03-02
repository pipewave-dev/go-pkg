package obs

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/pipewave-dev/go-pkg/pkg/observer/fieldvalue"
	"go.opentelemetry.io/otel/attribute"
)

func fieldsToAttribute(fields []fieldvalue.Field) []attribute.KeyValue {
	attrs := make([]attribute.KeyValue, 0, len(fields))
	for _, f := range fields {
		//nolint:exhaustive,errcheck
		switch f.Type {
		case fieldvalue.StringType:
			attrs = append(attrs, attribute.String(f.Key, f.Value.(string)))
		case fieldvalue.IntType:
			attrs = append(attrs, attribute.Int(f.Key, f.Value.(int)))
		case fieldvalue.Int64Type:
			attrs = append(attrs, attribute.Int64(f.Key, f.Value.(int64)))
		case fieldvalue.Float64Type:
			attrs = append(attrs, attribute.Float64(f.Key, f.Value.(float64)))
		case fieldvalue.BoolType:
			attrs = append(attrs, attribute.Bool(f.Key, f.Value.(bool)))
		case fieldvalue.ErrorType:
			if err, ok := f.Value.(error); ok && err != nil {
				attrs = append(attrs, attribute.String(f.Key, err.Error()))
			}
			attrs = append(attrs, attribute.String(f.Key, ""))
		default:
			attrs = append(attrs, attribute.String(f.Key, fmt.Sprintf("%v", f.Value)))
		}
	}
	return attrs
}

func fieldsToSlogAttrs(fields []fieldvalue.Field) []any {
	attrs := make([]any, 0, len(fields))
	for _, f := range fields {
		switch f.Type {
		case fieldvalue.StringType:
			attrs = append(attrs, slog.String(f.Key, f.Value.(string)))
		case fieldvalue.IntType:
			attrs = append(attrs, slog.Int(f.Key, f.Value.(int)))
		case fieldvalue.Int64Type:
			attrs = append(attrs, slog.Int64(f.Key, f.Value.(int64)))
		case fieldvalue.Float64Type:
			attrs = append(attrs, slog.Float64(f.Key, f.Value.(float64)))
		case fieldvalue.BoolType:
			attrs = append(attrs, slog.Bool(f.Key, f.Value.(bool)))
		case fieldvalue.DurationType:
			attrs = append(attrs, slog.Duration(f.Key, f.Value.(time.Duration)))
		case fieldvalue.TimeType:
			attrs = append(attrs, slog.Time(f.Key, f.Value.(time.Time)))
		case fieldvalue.ErrorType:
			if err, ok := f.Value.(error); ok && err != nil {
				attrs = append(attrs, slog.String(f.Key, err.Error()))
			} else {
				attrs = append(attrs, slog.String(f.Key, ""))
			}
		case fieldvalue.AnyType:
			attrs = append(attrs, slog.Any(f.Key, f.Value))
		}
	}
	return attrs
}
