package configprovider

// globalEnvT contains all application configuration loaded from YAML files and environment variables
type globalEnvT struct {
	Env           string `koanf:"ENV_NAME"`
	PodName       string `koanf:"POD_NAME"`
	ContainerID   string `koanf:"CONTAINER_ID"`
	Version       string `koanf:"VERSION"`
	AutoMigration bool   `koanf:"AUTO_MIGRATION"`

	ActiveConnection ActiveConnectionT `koanf:"ACTIVE_CONNECTION"`
	PingChecker      PingCheckerT      `koanf:"PING_CHECKER"`

	RateLimiter RateLimiterT `koanf:"RATE_LIMITER"`
	WorkerPool  WorkerPoolT  `koanf:"WORKER_POOL"`

	TraceIDHeader string      `koanf:"TRACE_ID_HEADER"`
	IpHeader      string      `koanf:"IP_HEADER"`
	Cors          CorsConfigT `koanf:"CORS"`

	Otel OtelT `koanf:"OTEL"`

	Valkey ValkeyT `koanf:"VALKEY"`

	DynamoDB DynamoConfigT `koanf:"DYNAMODB"`
	Postgres PostgresT     `koanf:"POSTGRES"`

	Fns *Fns
}
