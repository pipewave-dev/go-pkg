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
	e.ensureNonNil()

	e.Info.loadDefault()
	e.ActiveConnection.loadDefault()
	e.PingChecker.loadDefault()
	e.RateLimiter.loadDefault()
	e.WorkerPool.loadDefault()
	e.Otel.loadDefault()
}

// ensureNonNil allocates zero-value sub-configs for any block missing from
// the loaded YAML/env config, so Validate()/LoadDefault() never dereference
// a nil pointer receiver.
func (e *EnvType) ensureNonNil() {
	if e.Info == nil {
		e.Info = &InfoT{}
	}
	if e.ActiveConnection == nil {
		e.ActiveConnection = &ActiveConnectionT{}
	}
	if e.PingChecker == nil {
		e.PingChecker = &PingCheckerT{}
	}
	if e.RateLimiter == nil {
		e.RateLimiter = &RateLimiterT{}
	}
	if e.WorkerPool == nil {
		e.WorkerPool = &WorkerPoolT{}
	}
	if e.ExtractHeader == nil {
		e.ExtractHeader = &ExtractHeaderT{}
	}
	if e.Cors == nil {
		e.Cors = &CorsT{}
	}
	if e.Otel == nil {
		e.Otel = &OtelT{}
	}
	if e.Valkey == nil {
		e.Valkey = &ValkeyT{}
	}
	if e.DynamoDB == nil {
		e.DynamoDB = &DynamoConfigT{}
	}
	if e.Postgres == nil {
		e.Postgres = &PostgresT{}
	}
}
