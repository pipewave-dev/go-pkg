package configprovider

import (
	"time"
)

type EnvType struct {
	Env     string
	PodName string
	Version string

	Debug struct {
		Enabled bool
	}

	RateLimiter RateLimiterT

	WorkerPool WorkerPoolT

	TimeLocation *time.Location

	TraceIDHeader string
	IpHeader      string
	Cors          CorsConfig

	Otel     OtelT
	Valkey   ValkeyT
	DynamoDB DynamoConfigT

	Postgres PostgresT
}

func FromGoStruct(input EnvType) ConfigStore {
	env := globalEnvT{
		Env:           input.Env,
		PodName:       input.PodName,
		Version:       input.Version,
		WorkerPool:    input.WorkerPool,
		TimeLocation:  input.TimeLocation,
		TraceIDHeader: input.TraceIDHeader,
		IpHeader:      input.IpHeader,
		Cors:          input.Cors,
		Otel:          input.Otel,
		RateLimiter:   input.RateLimiter,
		Valkey:        input.Valkey,
		DynamoDB:      input.DynamoDB,
		Postgres:      input.Postgres,
	}

	// Mirror what loadDefault() does for timezone
	if input.TimeLocation != nil {
		time.Local = input.TimeLocation
	}

	env.validate()

	return &configStore{
		env: &env,
	}
}
