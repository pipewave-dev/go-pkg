package configprovider

import (
	"time"

	"github.com/samber/lo"
)

type CorsConfig struct {
	Enabled        bool     `koanf:"ENABLED"`
	ExactlyOrigins []string `koanf:"EXACTLY_ORIGINS"`
	RegexOrigins   []string `koanf:"REGEX_ORIGINS"`
}

/*
	ActiveConnectionT

How it works:
- Whenever a new connection is established, we create an active connection record in the database with the current timestamp as the last heartbeat time.
- The client is expected to send heartbeat messages at regular intervals (e.g. every 30 seconds) to update the last heartbeat time in the database.
- When checking for active connections, we compare the current time with the last heartbeat time. If the difference exceeds the HeartbeatCutoff, we consider the connection as dead and ignore it.
- For pending messages, when a message is sent to a user, we check the last
Known limitation:
- In some specific edge cases, it's possible that a connection is considered active (i.e. not cleaned up by the cronjob) but is actually dead (e.g. due to network issues). This is because the cleanup relies on the heartbeat cutoff, which may not be perfectly in sync with the actual connection state. To mitigate this, we can consider implementing a hybrid approach where we also check for connection liveness using Ping messages, in addition to relying on the heartbeat cutoff. This way, we can detect dead connections more proactively and reduce reliance on the cronjob to clean up old heartbeats.
*/
type ActiveConnectionT struct {
	HeartbeatCutoff time.Duration `koanf:"HEARTBEAT_CUTOFF"`
	PendingMsgTTL   time.Duration `koanf:"PENDING_MSG_TTL"`
}

type PingCheckerT struct {
	PingIdleAfter time.Duration `koanf:"HEARTBEAT_CUTOFF"`
	PongTimeout   time.Duration `koanf:"PENDING_MSG_TTL"`
}

func (m ActiveConnectionT) Validate() {
	if m.HeartbeatCutoff <= 0 {
		panic("active connection heartbeat cutoff must be greater than 0")
	}
	if m.PendingMsgTTL < m.HeartbeatCutoff {
		panic("active connection pending message ttl must be greater than heartbeat cutoff")
	}
}

type WorkerPoolT struct {
	Buffer         int `koanf:"BUFFER"`
	UpperThreshold int `koanf:"UPPER_THRESHOLD"`
	LowerThreshold int `koanf:"LOWER_THRESHOLD"`
}

type RateLimiterT struct {
	UserRate  int `koanf:"USER_RATE"`
	UserBurst int `koanf:"USER_BURST"`

	AnonymousRate  int `koanf:"ANONYMOUS_RATE"`
	AnonymousBurst int `koanf:"ANONYMOUS_BURST"`
}

func (r RateLimiterT) Validate() {
	if r.UserRate <= 0 {
		panic("rate limiter user rate must be greater than 0")
	}
	if r.UserBurst < r.UserRate {
		panic("rate limiter user burst must be greater than or equal to user rate")
	}
	if r.AnonymousRate <= 0 {
		panic("rate limiter anonymous rate must be greater than 0")
	}
	if r.AnonymousBurst < r.AnonymousRate {
		panic("rate limiter anonymous burst must be greater than or equal to anonymous rate")
	}
}

type ValkeyT struct {
	PrimaryAddress string `koanf:"PRIMARY_ADDRESS"`
	ReplicaAddress string `koanf:"REPLICA_ADDRESS"`
	Password       string `koanf:"PASSWORD"`
	DatabaseIdx    int    `koanf:"DB_INDEX"`
}

type DynamoConfigT struct {
	CreateTables    bool    `koanf:"CREATE_TABLES"`
	Region          string  `koanf:"REGION"`
	Endpoint        *string `koanf:"ENDPOINT"`
	Role            *string `koanf:"ROLE"`
	Profile         *string `koanf:"PROFILE"`
	StaticAccessKey *string `koanf:"STATIC_ACCESS_KEY"`
	StaticSecretKey *string `koanf:"STATIC_SECRET_KEY"`
	Tables          DynamoTables
}

// PostgresT contains PostgreSQL connection configuration
type PostgresT struct {
	CreateTables bool   `koanf:"CREATE_TABLES"`
	Host         string `koanf:"HOST"`
	Port         int    `koanf:"PORT"`
	DBName       string `koanf:"DB_NAME"`
	User         string `koanf:"USER"`
	Password     string `koanf:"PASSWORD"`
	SSLMode      string `koanf:"SSL_MODE"` // allow value: disable, require
	MaxConns     int32  `koanf:"MAX_CONNS"`
	MinConns     int32  `koanf:"MIN_CONNS"`
}

// KeySetT contains versioned key set configuration
type KeySetT struct {
	CurrentVersion int8     `koanf:"CURRENT_VERSION"`
	KeySetStr      []string `koanf:"KEY_SET"`
}

// GetKeySet returns the current version and key set as a map
func (ki KeySetT) GetKeySet() (currentVersion int8, keySet map[int8][]byte) {
	result := make(map[int8][]byte, len(ki.KeySetStr))
	for ver, keyStr := range ki.KeySetStr {
		if len(keyStr) < 32 {
			panic("cipher key must be 32 characters")
		}
		var b [32]byte
		copy(b[:], keyStr)
		result[(int8(ver))] = b[:]
	}
	return ki.CurrentVersion, result
}

// OtelT contains OpenTelemetry configuration
type OtelT struct {
	Enabled             bool   `koanf:"ENABLED"`
	AutoInstrumentation bool   `koanf:"AUTO_INSTRUMENTATION"`
	ExporterType        string `koanf:"EXPORTER_TYPE"`
	FilePath            string `koanf:"FILE_PATH"`
	CollectorEndpoint   string `koanf:"COLLECTOR_ENDPOINT"`
	CollectorInsecure   bool   `koanf:"COLLECTOR_INSECURE"`
}

// Validate checks if the OtelT configuration is valid
func (o *OtelT) Validate() {
	if o.Enabled {
		allowExporterType := []string{"discard", "stdout", "file", "otlp-grpc", "otlp-http"}
		if !lo.Contains(allowExporterType, o.ExporterType) {
			panic("OTEL.EXPORTER_TYPE must be one of [discard, stdout, file, otlp-grpc, otlp-http]")
		}
		if o.ExporterType == "file" && o.FilePath == "" {
			panic("OTEL.FILE_PATH is required when OTEL.EXPORTER_TYPE is file")
		}
		if o.ExporterType == "otlp-grpc" && o.CollectorEndpoint == "" {
			panic("OTEL.COLLECTOR_ENDPOINT is required when OTEL.EXPORTER_TYPE is otlp-grpc")
		}
		if o.ExporterType == "otlp-http" && o.CollectorEndpoint == "" {
			panic("OTEL.COLLECTOR_ENDPOINT is required when OTEL.EXPORTER_TYPE is otlp-http")
		}
	}
}

// ExternalT contains external service configurations
type ExternalT struct {
	Userbox struct {
		BaseUrl   string `koanf:"BASE_URL"`
		TimeoutMs int    `koanf:"TIMEOUT_MS"`
		WrapLog   bool   `koanf:"WRAP_LOG"`
		WrapOtel  bool   `koanf:"WRAP_OTEL"`

		AuthorizationToken string `koanf:"AUTHORIZATION_TOKEN"`
	} `koanf:"USERBOX"`

	GoogleOAuth2 struct {
		TimeoutSeconds int `koanf:"TIMEOUT_SECONDS"`
	} `koanf:"GOOGLE_OAUTH2"`

	GoogleRecaptcha struct {
		SecretKeyV2    string `koanf:"SECRET_KEY_V2"`
		SecretKeyV3    string `koanf:"SECRET_KEY_V3"`
		VerifyURL      string `koanf:"VERIFY_URL"`
		TimeoutSeconds int    `koanf:"TIMEOUT_SECONDS"`
	} `koanf:"GOOGLE_RECAPTCHA"`

	HCaptcha struct {
		SecretKey      string `koanf:"SECRET_KEY"`
		SiteKey        string `koanf:"SITE_KEY"`
		VerifyURL      string `koanf:"VERIFY_URL"`
		TimeoutSeconds int    `koanf:"TIMEOUT_SECONDS"`
		RemapDisabled  bool   `koanf:"REMAP_DISABLED"`
	} `koanf:"HCAPTCHA"`

	Turnstile struct {
		SecretKey      string `koanf:"SECRET_KEY"`
		VerifyURL      string `koanf:"VERIFY_URL"`
		TimeoutSeconds int    `koanf:"TIMEOUT_SECONDS"`
		Mock           struct {
			Enabled bool `koanf:"ENABLED"`
			Result  int  `koanf:"RESULT"`
		} `koanf:"MOCK"`
	} `koanf:"TURNSTILE"`
}

// DynamoTables contains DynamoDB table name configurations
type DynamoTables struct {
	ActiveConnection string `koanf:"ACTIVE_CONNECTION"`
	FcmDevice        string `koanf:"FCM_DEVICE"`
	Group            string `koanf:"GROUP"`
	User             string `koanf:"USER"`
	UserGroup        string `koanf:"USER_GROUP"`
	NotiContent      string `koanf:"NOTI_CONTENT"`
	GNoti            string `koanf:"G_NOTI"`
	UNoti            string `koanf:"U_NOTI"`
	NotiTimeBucket   string `koanf:"NOTI_TIME_BUCKET"`
	PendingMessage   string `koanf:"PENDING_MESSAGE"`
}
