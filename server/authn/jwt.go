package authn

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type JWTConfig struct {
	JWKSURL          string
	PublicKeyPEMFile string
	UserIDClaim      string
	MetadataClaims   []string
}

// JWTInspector implements the types.Fns.InspectToken contract by
// verifying a JWT locally — no callback round-trip per connection.
type JWTInspector struct {
	keyfunc jwt.Keyfunc
	cfg     JWTConfig
}

var allowedAlgs = []string{"RS256", "RS384", "RS512", "ES256", "ES384", "ES512", "EdDSA"}

// NewJWTInspector builds an inspector from a JWKS URL (refreshed in the
// background, bound to ctx) or a static PKIX public key PEM file.
func NewJWTInspector(ctx context.Context, cfg JWTConfig) (*JWTInspector, error) {
	if cfg.UserIDClaim == "" {
		cfg.UserIDClaim = "sub"
	}

	var kf jwt.Keyfunc
	switch {
	case cfg.JWKSURL != "":
		k, err := keyfunc.NewDefaultCtx(ctx, []string{cfg.JWKSURL})
		if err != nil {
			return nil, fmt.Errorf("authn: init JWKS from %s: %w", cfg.JWKSURL, err)
		}
		kf = k.Keyfunc
	case cfg.PublicKeyPEMFile != "":
		pub, err := loadPKIXPublicKey(cfg.PublicKeyPEMFile)
		if err != nil {
			return nil, err
		}
		kf = func(*jwt.Token) (any, error) { return pub, nil }
	default:
		return nil, errors.New("authn: JWTConfig requires JWKSURL or PublicKeyPEMFile")
	}

	return &JWTInspector{keyfunc: kf, cfg: cfg}, nil
}

func loadPKIXPublicKey(pemFile string) (any, error) {
	raw, err := os.ReadFile(pemFile)
	if err != nil {
		return nil, fmt.Errorf("authn: read public key file %s: %w", pemFile, err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("authn: %s is not PEM", pemFile)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("authn: parse PKIX public key from %s: %w", pemFile, err)
	}
	return pub, nil
}

// InspectToken satisfies the types.Fns.InspectToken signature.
func (j *JWTInspector) InspectToken(_ context.Context, token string, _ http.Header) (string, bool, map[string]string, error) {
	token = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "Bearer "))

	claims := jwt.MapClaims{}
	_, err := jwt.ParseWithClaims(token, claims, j.keyfunc,
		jwt.WithValidMethods(allowedAlgs),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return "", false, nil, fmt.Errorf("authn: verify jwt: %w", err)
	}

	userID, _ := claims[j.cfg.UserIDClaim].(string)
	if userID == "" {
		return "", false, nil, fmt.Errorf("authn: claim %q missing or not a string", j.cfg.UserIDClaim)
	}

	var metadata map[string]string
	for _, name := range j.cfg.MetadataClaims {
		if v, ok := claims[name].(string); ok {
			if metadata == nil {
				metadata = map[string]string{}
			}
			metadata[name] = v
		}
	}
	return userID, false, metadata, nil
}
