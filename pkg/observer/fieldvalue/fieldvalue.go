package fieldvalue

import "time"

type Field struct {
	Key   string
	Value any
	Type  FieldType
}

type FieldType int

const (
	StringType FieldType = iota
	IntType
	Int64Type
	Float64Type
	BoolType
	DurationType
	TimeType
	ErrorType
	AnyType
)

// Field constructors
func String(key, value string) Field {
	return Field{key, value, StringType}
}

func Int(key string, value int) Field {
	return Field{key, value, IntType}
}

func Int64(key string, value int64) Field {
	return Field{key, value, Int64Type}
}

func Float64(key string, value float64) Field {
	return Field{key, value, Float64Type}
}

func Bool(key string, value bool) Field {
	return Field{key, value, BoolType}
}

func Duration(key string, value time.Duration) Field {
	return Field{key, value, DurationType}
}

func Time(key string, value time.Time) Field {
	return Field{key, value, TimeType}
}

func Error(key string, value error) Field {
	if value == nil {
		return Field{key, "", ErrorType}
	}
	return Field{key, value, ErrorType}
}

func Any(key string, value any) Field {
	return Field{key, value, AnyType}
}
