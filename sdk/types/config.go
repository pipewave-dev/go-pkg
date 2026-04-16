package types

type EnvType struct {
	Info *InfoT `koanf:"INFO"`

	AutoMigration bool `koanf:"AUTO_MIGRATION"`

	ActiveConnection *ActiveConnectionT `koanf:"ACTIVE_CONNECTION"`
	PingChecker      *PingCheckerT      `koanf:"PING_CHECKER"`

	RateLimiter *RateLimiterT `koanf:"RATE_LIMITER"`

	WorkerPool *WorkerPoolT `koanf:"WORKER_POOL"`

	ExtractHeader *ExtractHeaderT `koanf:"EXTRACT_HEADER"`
	Cors          *CorsT          `koanf:"CORS"`

	Otel     *OtelT         `koanf:"OTEL"`
	Valkey   *ValkeyT       `koanf:"VALKEY"`
	DynamoDB *DynamoConfigT `koanf:"DYNAMODB"`

	Postgres *PostgresT `koanf:"POSTGRES"`
}

func (e *EnvType) Validate() {
	e.Info.validate()
	e.Cors.validate()
	e.ActiveConnection.validate()
	e.PingChecker.validate()
	e.RateLimiter.validate()
	e.Otel.validate()
	e.WorkerPool.validate()
}

func (e *EnvType) LoadDefault() {
	e.Info.loadDefault()
	e.ActiveConnection.loadDefault()
	e.PingChecker.loadDefault()
	e.RateLimiter.loadDefault()
	e.WorkerPool.loadDefault()
}
