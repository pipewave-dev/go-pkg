package delivery

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pipewave-dev/go-pkg/shared/utils/fn"
)

// anonymousInstanceMaxAge bounds how long a server-issued anonymous InstanceID
// stays valid, limiting the resume window (and exposure if a token ever leaks,
// e.g. via logs) — mirrors the earlier cookie MaxAge design.
const anonymousInstanceMaxAge = 6 * time.Hour

// anonymousInstanceSigner mints and verifies opaque, unforgeable anonymous
// InstanceIDs of the form "<nanoid>:<unixMintTime>.<base64url(hmac-sha256)>".
//
// This replaces both the legacy X-Pipewave-ID header (client-controlled,
// unauthenticated — the original vulnerability) and an HttpOnly-cookie-based
// design considered earlier: cookies don't survive this SDK's primary
// deployment model, where a customer's frontend origin calls a different
// Pipewave API origin (cross-site — SameSite cookies aren't sent). A signed
// token is just an ordinary app-level header value, so it works identically
// same-site or cross-site. See ai-feedback/06-anonymous-ratelimit-session-bypass.md.
type anonymousInstanceSigner struct {
	secret []byte
}

func newAnonymousInstanceSigner(secret string) *anonymousInstanceSigner {
	return &anonymousInstanceSigner{secret: []byte(secret)}
}

// mintOrReadAnonymousInstanceID is the sole mint point for anonymous
// InstanceIDs. Only IssueTmpToken calls this; every other anonymous
// entrypoint (/lp, /lp-send) must already carry a token minted here (see
// readAnonymousInstanceID) — so IP throttling applied at /issue-tmp-token
// bounds how fast an attacker can churn through fresh anonymous instances,
// regardless of transport.
func (s *anonymousInstanceSigner) mintOrReadAnonymousInstanceID(r *http.Request) string {
	if token, ok := s.readAnonymousInstanceID(r); ok {
		return token
	}
	payload := fn.NewNanoID() + ":" + strconv.FormatInt(time.Now().Unix(), 10)
	return s.sign(payload)
}

// readAnonymousInstanceID verifies X-Pipewave-ID as a token this signer
// minted. Deliberately does not trust the header value at face value — that
// is exactly what let an attacker free-mint or replay another session's ID.
func (s *anonymousInstanceSigner) readAnonymousInstanceID(r *http.Request) (token string, ok bool) {
	v := r.Header.Get("X-Pipewave-ID")
	if v == "" || !s.verify(v) {
		return "", false
	}
	return v, true
}

func (s *anonymousInstanceSigner) sign(payload string) string {
	return payload + "." + s.mac(payload)
}

func (s *anonymousInstanceSigner) mac(payload string) string {
	h := hmac.New(sha256.New, s.secret)
	h.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func (s *anonymousInstanceSigner) verify(token string) bool {
	payload, sig, found := strings.Cut(token, ".")
	if !found || payload == "" || sig == "" {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(sig), []byte(s.mac(payload))) != 1 {
		return false
	}

	_, mintedAtStr, found := strings.Cut(payload, ":")
	if !found {
		return false
	}
	mintedAt, err := strconv.ParseInt(mintedAtStr, 10, 64)
	if err != nil {
		return false
	}
	return time.Since(time.Unix(mintedAt, 0)) <= anonymousInstanceMaxAge
}
