// server/config/config.go
package serverconfig

import (
	"fmt"
	"path/filepath"
	"time"

	koanfpvd "github.com/pipewave-dev/go-pkg/pkg/koanf"
)

const (
	AuthModeJWT     = "jwt"
	AuthModeWebhook = "webhook"

	HandleMsgModeSync     = "sync"
	HandleMsgModeForward  = "forward"
	HandleMsgModeDisabled = "disabled"

	SignatureModeEnabled  = "enabled"
	SignatureModeDisabled = "disabled"

	RepositoryPostgres = "postgres"
	RepositoryDynamoDB = "dynamodb"

	UnhealthyActionShutdown = "shutdown"
	UnhealthyActionLogOnly  = "log-only"

	TransportWebhook = "webhook"
	TransportPubsub  = "pubsub"

	PubsubDriverNATSJS = "natsjs"
)

type ServerConfigT struct {
	ClientAddr string     `koanf:"CLIENT_ADDR"`
	AdminAddr  string     `koanf:"ADMIN_ADDR"`
	APIKeys    []string   `koanf:"API_KEYS"`
	Repository string     `koanf:"REPOSITORY"`
	Auth       AuthT      `koanf:"AUTH"`
	Callbacks  CallbacksT `koanf:"CALLBACKS"`
}

type AuthT struct {
	Mode string `koanf:"MODE"` // jwt | webhook
	JWT  JWTT   `koanf:"JWT"`
}

type JWTT struct {
	JWKSURL          string   `koanf:"JWKS_URL"`
	PublicKeyPEMFile string   `koanf:"PUBLIC_KEY_PEM_FILE"`
	UserIDClaim      string   `koanf:"USER_ID_CLAIM"`
	MetadataClaims   []string `koanf:"METADATA_CLAIMS"`
}

type CallbacksT struct {
	BaseURL       string        `koanf:"BASE_URL"`
	Signature     SignatureT    `koanf:"SIGNATURE"`
	HandleMessage HandleMsgT    `koanf:"HANDLE_MESSAGE"`
	SyncTimeout   time.Duration `koanf:"SYNC_TIMEOUT"`
	AsyncRetryMax int           `koanf:"ASYNC_RETRY_MAX"`

	AsyncBackoff        []time.Duration `koanf:"ASYNC_BACKOFF"`
	SyncRetry           SyncRetryT      `koanf:"SYNC_RETRY"`
	Breaker             BreakerT        `koanf:"BREAKER"`
	Ping                PingT           `koanf:"PING"`
	UnhealthyAction     string          `koanf:"UNHEALTHY_ACTION"`
	BreakerOpenShutdown time.Duration   `koanf:"BREAKER_OPEN_SHUTDOWN"`
	Transport           string          `koanf:"TRANSPORT"`
	Pubsub              CallbackPubsubT `koanf:"PUBSUB"`
}

type SyncRetryT struct {
	Max     int           `koanf:"MAX"`
	Backoff time.Duration `koanf:"BACKOFF"`
}

type BreakerT struct {
	Threshold int           `koanf:"THRESHOLD"`
	Cooldown  time.Duration `koanf:"COOLDOWN"`
}

type PingT struct {
	Enabled       bool          `koanf:"ENABLED"`
	Path          string        `koanf:"PATH"`
	Interval      time.Duration `koanf:"INTERVAL"`
	Timeout       time.Duration `koanf:"TIMEOUT"`
	BootCheck     bool          `koanf:"BOOT_CHECK"`
	FailThreshold int           `koanf:"FAIL_THRESHOLD"`
}

type SignatureT struct {
	Mode           string `koanf:"MODE"`
	SigningKeyFile string `koanf:"SIGNING_KEY_FILE"`
}

type HandleMsgT struct {
	Mode    string        `koanf:"MODE"` // sync | forward | disabled
	Timeout time.Duration `koanf:"TIMEOUT"`
}

// CallbackPubsubT cấu hình transport pubsub cho Class-2 events. Đây là
// instance RIÊNG, không liên quan tới pubsub fanout nội bộ giữa các node.
type CallbackPubsubT struct {
	Driver        string `koanf:"DRIVER"`
	URL           string `koanf:"URL"`
	Stream        string `koanf:"STREAM"`
	SubjectPrefix string `koanf:"SUBJECT_PREFIX"`
}

type rootT struct {
	Server ServerConfigT `koanf:"SERVER"`
}

// Load reads the SERVER section from the given YAML files (later files
// override earlier ones), merges APP_-prefixed env vars on top, applies
// defaults, and validates. It intentionally reuses pkg/koanf so the server
// section lives in the same files as the core EnvType config.
func Load(yamlFiles []string) (*ServerConfigT, error) {
	koanfYamlFiles := make([]struct {
		FileDir   string
		FilePath  string
		SkipError bool
	}, 0, len(yamlFiles))
	for _, filePath := range yamlFiles {
		// Split absolute paths into directory and filename for koanf
		dir := filepath.Dir(filePath)
		file := filepath.Base(filePath)
		koanfYamlFiles = append(koanfYamlFiles, struct {
			FileDir   string
			FilePath  string
			SkipError bool
		}{FileDir: dir, FilePath: file})
	}

	k := koanfpvd.NewKoanfProvider(&koanfpvd.KoanfConfig{
		YamlConfigFile: koanfYamlFiles,
		EnvPrefix:      "APP",
	})

	var root rootT
	if err := k.Unmarshall(&root); err != nil {
		return nil, fmt.Errorf("serverconfig: unmarshal: %w", err)
	}

	cfg := root.Server
	cfg.loadDefault()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *ServerConfigT) loadDefault() {
	if c.ClientAddr == "" {
		c.ClientAddr = ":8080"
	}
	if c.AdminAddr == "" {
		c.AdminAddr = ":8081"
	}
	if c.Repository == "" {
		c.Repository = RepositoryPostgres
	}
	if c.Auth.JWT.UserIDClaim == "" {
		c.Auth.JWT.UserIDClaim = "sub"
	}
	if c.Callbacks.Signature.Mode == "" {
		c.Callbacks.Signature.Mode = SignatureModeEnabled
	}
	if c.Callbacks.Signature.SigningKeyFile == "" {
		c.Callbacks.Signature.SigningKeyFile = "webhook_ed25519.key"
	}
	if c.Callbacks.HandleMessage.Mode == "" {
		c.Callbacks.HandleMessage.Mode = HandleMsgModeSync
	}
	if c.Callbacks.HandleMessage.Timeout <= 0 {
		c.Callbacks.HandleMessage.Timeout = 5 * time.Second
	}
	if c.Callbacks.SyncTimeout <= 0 {
		c.Callbacks.SyncTimeout = 3 * time.Second
	}
	if c.Callbacks.AsyncRetryMax <= 0 {
		c.Callbacks.AsyncRetryMax = 6
	}
	if c.Callbacks.SyncRetry.Max <= 0 {
		c.Callbacks.SyncRetry.Max = 1
	}
	if c.Callbacks.SyncRetry.Backoff <= 0 {
		c.Callbacks.SyncRetry.Backoff = 100 * time.Millisecond
	}
	if c.Callbacks.Breaker.Threshold <= 0 {
		c.Callbacks.Breaker.Threshold = 5
	}
	if c.Callbacks.Breaker.Cooldown <= 0 {
		c.Callbacks.Breaker.Cooldown = 10 * time.Second
	}
	if c.Callbacks.Ping.Enabled {
		if c.Callbacks.Ping.Path == "" {
			c.Callbacks.Ping.Path = "/pipewave/ping"
		}
		if c.Callbacks.Ping.Interval <= 0 {
			c.Callbacks.Ping.Interval = 30 * time.Second
		}
		if c.Callbacks.Ping.Timeout <= 0 {
			c.Callbacks.Ping.Timeout = 3 * time.Second
		}
		if c.Callbacks.Ping.FailThreshold <= 0 {
			c.Callbacks.Ping.FailThreshold = 3
		}
		c.Callbacks.Ping.BootCheck = true // luôn boot-check khi ping enabled
	}
	if c.Callbacks.UnhealthyAction == "" {
		c.Callbacks.UnhealthyAction = UnhealthyActionLogOnly
	}
	if c.Callbacks.Transport == "" {
		c.Callbacks.Transport = TransportWebhook
	}
	if c.Callbacks.Transport == TransportPubsub {
		if c.Callbacks.Pubsub.Driver == "" {
			c.Callbacks.Pubsub.Driver = PubsubDriverNATSJS
		}
		if c.Callbacks.Pubsub.Stream == "" {
			c.Callbacks.Pubsub.Stream = "PIPEWAVE_EVENTS"
		}
		if c.Callbacks.Pubsub.SubjectPrefix == "" {
			c.Callbacks.Pubsub.SubjectPrefix = "pipewave.events"
		}
	}
}

func (c *ServerConfigT) validate() error {
	if len(c.APIKeys) == 0 {
		return fmt.Errorf("serverconfig: SERVER.API_KEYS must not be empty")
	}
	switch c.Auth.Mode {
	case AuthModeWebhook:
	case AuthModeJWT:
		if c.Auth.JWT.JWKSURL == "" && c.Auth.JWT.PublicKeyPEMFile == "" {
			return fmt.Errorf("serverconfig: SERVER.AUTH.MODE=jwt requires JWKS_URL or PUBLIC_KEY_PEM_FILE")
		}
	default:
		return fmt.Errorf("serverconfig: SERVER.AUTH.MODE must be %q or %q, got %q", AuthModeJWT, AuthModeWebhook, c.Auth.Mode)
	}
	if c.Callbacks.BaseURL == "" {
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.BASE_URL is required")
	}
	switch c.Callbacks.Transport {
	case TransportWebhook:
	case TransportPubsub:
		if c.Callbacks.Pubsub.URL == "" {
			return fmt.Errorf("serverconfig: SERVER.CALLBACKS.PUBSUB.URL is required when TRANSPORT=pubsub")
		}
		if c.Callbacks.Pubsub.Driver != PubsubDriverNATSJS {
			return fmt.Errorf("serverconfig: SERVER.CALLBACKS.PUBSUB.DRIVER must be %q, got %q",
				PubsubDriverNATSJS, c.Callbacks.Pubsub.Driver)
		}
	default:
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.TRANSPORT must be %q or %q, got %q",
			TransportWebhook, TransportPubsub, c.Callbacks.Transport)
	}
	switch c.Callbacks.HandleMessage.Mode {
	case HandleMsgModeSync, HandleMsgModeForward, HandleMsgModeDisabled:
	default:
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.HANDLE_MESSAGE.MODE must be sync|forward|disabled, got %q", c.Callbacks.HandleMessage.Mode)
	}
	switch c.Callbacks.Signature.Mode {
	case SignatureModeEnabled, SignatureModeDisabled:
	default:
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.SIGNATURE.MODE must be %q or %q, got %q", SignatureModeEnabled, SignatureModeDisabled, c.Callbacks.Signature.Mode)
	}
	switch c.Repository {
	case RepositoryPostgres, RepositoryDynamoDB:
	default:
		return fmt.Errorf("serverconfig: SERVER.REPOSITORY must be postgres|dynamodb, got %q", c.Repository)
	}
	switch c.Callbacks.UnhealthyAction {
	case UnhealthyActionShutdown, UnhealthyActionLogOnly:
	default:
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.UNHEALTHY_ACTION must be %q or %q, got %q",
			UnhealthyActionShutdown, UnhealthyActionLogOnly, c.Callbacks.UnhealthyAction)
	}
	if c.Callbacks.SyncRetry.Max < 1 {
		return fmt.Errorf("serverconfig: SERVER.CALLBACKS.SYNC_RETRY.MAX must be >= 1, got %d", c.Callbacks.SyncRetry.Max)
	}
	return nil
}
