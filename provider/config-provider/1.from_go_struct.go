package configprovider

type EnvType struct {
	Env     string
	PodName string

	Debug struct {
		Enabled bool
	}

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
		Env:           input.Env,
		PodName:       input.PodName,
		WorkerPool:    input.WorkerPool,
		TraceIDHeader: input.TraceIDHeader,
		IpHeader:      input.IpHeader,
		Cors:          input.Cors,
		Otel:          input.Otel,
		RateLimiter:   input.RateLimiter,
		Valkey:        input.Valkey,
		DynamoDB:      input.DynamoDB,
		Postgres:      input.Postgres,
	}

	env.validate()

	return &configStore{
		env: &env,
	}
}
