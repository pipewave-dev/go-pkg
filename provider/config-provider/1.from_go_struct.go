package configprovider

import "time"

type EnvType struct {
	Env         string
	PodName     string
	ContainerID string

	HeartbeatCutoff time.Duration

	MessageHub MessageHubT

	RateLimiter RateLimiterT

	WorkerPool WorkerPoolT

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
		Env:             input.Env,
		PodName:         input.PodName,
		ContainerID:     input.ContainerID,
		HeartbeatCutoff: input.HeartbeatCutoff,
		MessageHub:      input.MessageHub,
		WorkerPool:      input.WorkerPool,
		TraceIDHeader:   input.TraceIDHeader,
		IpHeader:        input.IpHeader,
		Cors:            input.Cors,
		Otel:            input.Otel,
		RateLimiter:     input.RateLimiter,
		Valkey:          input.Valkey,
		DynamoDB:        input.DynamoDB,
		Postgres:        input.Postgres,
	}

	env.loadDefault()
	env.validate()

	return &configStore{
		env: &env,
	}
}
