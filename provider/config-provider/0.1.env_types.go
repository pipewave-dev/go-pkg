package configprovider

import (
	"time"
)

// globalEnvT contains all application configuration loaded from YAML files and environment variables
type globalEnvT struct {
	Env     string `koanf:"ENV_NAME"`
	PodName string `koanf:"POD_NAME"`
	Version string `koanf:"VERSION"`

	WorkerPool WorkerPoolT `koanf:"WORKER_POOL"`

	/*
		See
		https://en.wikipedia.org/wiki/List_of_tz_database_time_zones
	*/
	TimezoneStr  *string `koanf:"TIME_ZONE"`
	TimeLocation *time.Location

	TraceIDHeader string     `koanf:"TRACE_ID_HEADER"`
	IpHeader      string     `koanf:"IP_HEADER"`
	Cors          CorsConfig `koanf:"CORS"`

	Otel OtelT `koanf:"OTEL"`

	RateLimiter RateLimiterT `koanf:"RATE_LIMITER"`

	Valkey ValkeyT `koanf:"VALKEY"`

	DynamoDB DynamoConfigT `koanf:"DYNAMODB"`
	Postgres PostgresT     `koanf:"POSTGRES"`

	Fns *Fns
}
