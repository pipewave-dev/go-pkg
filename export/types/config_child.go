package types

import (
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/samber/lo"
)

type InfoT struct {
	Env         string `koanf:"ENV"`
	PodName     string `koanf:"POD_NAME"`
	ContainerID string `koanf:"CONTAINER_ID"`
}

func (r *InfoT) validate() {
	if r.Env == "" {
		panic("info env must not be empty")
	}
	if r.ContainerID == "" {
		panic("info container id must not be empty")
	}
}

func (r *InfoT) loadDefault() {
	if r.ContainerID == "" {
		r.ContainerID = generateContainerID()
	}
}

type ExtractHeaderT struct {
	TraceIDHeader string `koanf:"TRACE_ID_HEADER"`
	IpHeader      string `koanf:"IP_HEADER"`
}

type CorsT struct {
	Enabled        bool     `koanf:"ENABLED"`
	ExactlyOrigins []string `koanf:"EXACTLY_ORIGINS"`
	RegexOrigins   []string `koanf:"REGEX_ORIGINS"`

	compiledRegexOrigins []*regexp.Regexp
}

func (c *CorsT) validate() {
	if c.Enabled {
		if len(c.ExactlyOrigins) == 0 && len(c.RegexOrigins) == 0 {
			panic("cors config: either exactly origins or regex origins must be provided when cors is enabled")
		}
		c.compiledRegexOrigins = make([]*regexp.Regexp, 0, len(c.RegexOrigins))
		for _, pattern := range c.RegexOrigins {
			c.compiledRegexOrigins = append(c.compiledRegexOrigins, regexp.MustCompile("^(?:"+pattern+")$"))
		}
	}
}

// IsAllowedOrigin reports whether origin matches an exact allowlist entry or
// one of the (anchored) regex allowlist patterns.
func (c *CorsT) IsAllowedOrigin(origin string) bool {
	if slices.Contains(c.ExactlyOrigins, origin) {
		return true
	}
	for _, re := range c.compiledRegexOrigins {
		if re.MatchString(origin) {
			return true
		}
	}
	return false
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

func (m *ActiveConnectionT) validate() {
	if m.HeartbeatCutoff <= 0 {
		panic("active connection heartbeat cutoff must be greater than 0")
	}
	if m.PendingMsgTTL < m.HeartbeatCutoff {
		panic("active connection pending message ttl must be greater than heartbeat cutoff")
	}
}

func (m *ActiveConnectionT) loadDefault() {
	if m.HeartbeatCutoff == 0 {
		m.HeartbeatCutoff = 60 * time.Second
	}
	if m.PendingMsgTTL == 0 {
		m.PendingMsgTTL = m.HeartbeatCutoff * 2
	}
}

type PingCheckerT struct {
	PingIdleAfter time.Duration `koanf:"PING_IDLE_AFTER"`
	PongTimeout   time.Duration `koanf:"PONG_TIMEOUT"`
}

func (p *PingCheckerT) validate() {
	if p.PingIdleAfter <= 0 {
		panic("ping checker ping idle after must be greater than 0")
	}
	if p.PongTimeout <= 0 {
		panic("ping checker pong timeout must be greater than 0")
	}
}

func (p *PingCheckerT) loadDefault() {
	if p.PingIdleAfter == 0 {
		p.PingIdleAfter = 20 * time.Second
	}
	if p.PongTimeout == 0 {
		p.PongTimeout = 3 * time.Second
	}
}

type WorkerPoolT struct {
	Buffer         int `koanf:"BUFFER"`
	UpperThreshold int `koanf:"UPPER_THRESHOLD"`
	LowerThreshold int `koanf:"LOWER_THRESHOLD"`
}

func (w *WorkerPoolT) validate() {
	if w.Buffer <= 0 {
		panic("worker pool buffer must be greater than 0")
	}
	if w.UpperThreshold <= 0 {
		panic("worker pool upper threshold must be greater than 0")
	}
	if w.LowerThreshold < 0 {
		panic("worker pool lower threshold must be greater than or equal to 0")
	}
	if w.UpperThreshold <= w.LowerThreshold {
		panic("worker pool upper threshold must be greater than lower threshold")
	}
}

func (w *WorkerPoolT) loadDefault() {
	if w.Buffer == 0 {
		w.Buffer = 64
	}
	if w.UpperThreshold == 0 {
		w.UpperThreshold = 48
	}
	if w.LowerThreshold == 0 {
		w.LowerThreshold = 16
	}
}

type RateLimiterT struct {
	UserRate  int `koanf:"USER_RATE"`
	UserBurst int `koanf:"USER_BURST"`

	AnonymousRate  int `koanf:"ANONYMOUS_RATE"`
	AnonymousBurst int `koanf:"ANONYMOUS_BURST"`

	// ControlFrameRate/Burst limit ping frames per second, per-connection, at the
	// transport layer — independent of the application-level user/anonymous limiter —
	// so a client can't flood pong responses by flooding pings.
	ControlFrameRate  int `koanf:"CONTROL_FRAME_RATE"`
	ControlFrameBurst int `koanf:"CONTROL_FRAME_BURST"`

	// IssueTokenIPRate/Burst throttle POST /issue-tmp-token per client IP — the
	// sole mint point for anonymous InstanceIDs. Bounds how fast an attacker can
	// churn through fresh anonymous sessions even though InstanceID is no longer
	// client-chosen (see ai-feedback/06-anonymous-ratelimit-session-bypass.md).
	IssueTokenIPRate  int `koanf:"ISSUE_TOKEN_IP_RATE"`
	IssueTokenIPBurst int `koanf:"ISSUE_TOKEN_IP_BURST"`
}

func (r *RateLimiterT) validate() {
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
	if r.ControlFrameRate <= 0 {
		panic("rate limiter control frame rate must be greater than 0")
	}
	if r.ControlFrameBurst < r.ControlFrameRate {
		panic("rate limiter control frame burst must be greater than or equal to control frame rate")
	}
	if r.IssueTokenIPRate <= 0 {
		panic("rate limiter issue-token IP rate must be greater than 0")
	}
	if r.IssueTokenIPBurst < r.IssueTokenIPRate {
		panic("rate limiter issue-token IP burst must be greater than or equal to issue-token IP rate")
	}
}

func (r *RateLimiterT) loadDefault() {
	if r.UserRate == 0 {
		r.UserRate = 30
	}
	if r.UserBurst == 0 {
		r.UserBurst = 60
	}
	if r.AnonymousRate == 0 {
		r.AnonymousRate = 5
	}
	if r.AnonymousBurst == 0 {
		r.AnonymousBurst = 10
	}
	if r.ControlFrameRate == 0 {
		r.ControlFrameRate = 5
	}
	if r.ControlFrameBurst == 0 {
		r.ControlFrameBurst = 10
	}
	if r.IssueTokenIPRate == 0 {
		r.IssueTokenIPRate = 5
	}
	if r.IssueTokenIPBurst == 0 {
		r.IssueTokenIPBurst = 20
	}
}

// AnonymousInstanceT holds the HMAC secret used to sign/verify server-issued
// anonymous InstanceIDs (see core/service/websocket/mediator/delivery/anon_instance.go).
// Must be shared and stable across all replicas of the service, since a token
// minted by one replica must verify on whichever replica handles the next
// request. See ai-feedback/06-anonymous-ratelimit-session-bypass.md.
type AnonymousInstanceT struct {
	Secret string `koanf:"SECRET"`
}

func (a *AnonymousInstanceT) validate() {
	if a.Secret == "" {
		panic("anonymous instance secret must be set (ANONYMOUS_INSTANCE_SECRET)")
	}
}

type ValkeyT struct {
	PrimaryAddress string `koanf:"PRIMARY_ADDRESS"`
	ReplicaAddress string `koanf:"REPLICA_ADDRESS"`
	Password       string `koanf:"PASSWORD"`
	DatabaseIdx    int    `koanf:"DB_INDEX"`
}

type DynamoConfigT struct {
	CreateTables    bool         `koanf:"CREATE_TABLES"`
	Region          string       `koanf:"REGION"`
	Endpoint        *string      `koanf:"ENDPOINT"`
	Role            *string      `koanf:"ROLE"`
	Profile         *string      `koanf:"PROFILE"`
	StaticAccessKey *string      `koanf:"STATIC_ACCESS_KEY"`
	StaticSecretKey *string      `koanf:"STATIC_SECRET_KEY"`
	Tables          DynamoTables `koanf:"TABLES"`
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

// OtelT contains OpenTelemetry configuration
type OtelT struct {
	Enabled bool `koanf:"ENABLED"`
	// Debug = -4, Info = 0, Warn = 4, Error = 8
	LogLevel            int    `koanf:"LOG_LEVEL"`
	AutoInstrumentation bool   `koanf:"AUTO_INSTRUMENTATION"`
	ExporterType        string `koanf:"EXPORTER_TYPE"`
	FilePath            string `koanf:"FILE_PATH"`
	CollectorEndpoint   string `koanf:"COLLECTOR_ENDPOINT"`
	CollectorInsecure   bool   `koanf:"COLLECTOR_INSECURE"`
}

// Validate checks if the OtelT configuration is valid
func (o *OtelT) validate() {
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

func (o *OtelT) loadDefault() {
	if o.ExporterType == "" {
		o.ExporterType = "discard"
	}
}

// MetricsT contains Prometheus metrics configuration.
//
// Metrics are served on their own listener, separate from the client and admin
// listeners, so Prometheus can scrape without being granted admin API access.
type MetricsT struct {
	Enabled bool   `koanf:"ENABLED"`
	Port    int    `koanf:"PORT"`
	Path    string `koanf:"PATH"`
	// MsgTypeAllowlist names the app-level msg_type values that get their own
	// metric label. Anything not listed is reported as "other". Empty (the
	// default) collapses every app-level msg_type — msg_type is
	// client-controlled, so this list is what bounds label cardinality.
	MsgTypeAllowlist []string `koanf:"MSG_TYPE_ALLOWLIST"`
}

// maxMetricsMsgTypeLen must stay in sync with metrics.maxMsgTypeLen.
const maxMetricsMsgTypeLen = 32

func (m *MetricsT) validate() {
	if !m.Enabled {
		return
	}
	if m.Port <= 0 || m.Port > 65535 {
		panic("METRICS.PORT must be between 1 and 65535 when METRICS.ENABLED is true")
	}
	if !strings.HasPrefix(m.Path, "/") {
		panic("METRICS.PATH must start with '/' when METRICS.ENABLED is true")
	}
	for _, e := range m.MsgTypeAllowlist {
		if e == "" || len(e) > maxMetricsMsgTypeLen {
			panic("METRICS.MSG_TYPE_ALLOWLIST entries must be 1..32 characters")
		}
		for i := 0; i < len(e); i++ {
			if e[i] < 0x20 || e[i] > 0x7E {
				panic("METRICS.MSG_TYPE_ALLOWLIST entries must be printable ASCII")
			}
		}
	}
}

func (m *MetricsT) loadDefault() {
	if m.Port == 0 {
		m.Port = 9090
	}
	if m.Path == "" {
		m.Path = "/metrics"
	}
}

// ValidateForTest exposes validate() to the types_test package.
func (m *MetricsT) ValidateForTest() { m.validate() }

// LoadDefaultForTest exposes loadDefault() to the types_test package.
func (m *MetricsT) LoadDefaultForTest() { m.loadDefault() }

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
