package delivery

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMintOrReadAnonymousInstanceID_MintsWhenNoToken(t *testing.T) {
	s := newAnonymousInstanceSigner("test-secret")
	r := httptest.NewRequest(http.MethodPost, "/issue-tmp-token", nil)
	r.Header.Set("X-Pipewave-ID", "attacker-chosen-id") // unsigned, must be ignored

	token := s.mintOrReadAnonymousInstanceID(r)

	if token == "" {
		t.Fatal("expected a minted token, got empty string")
	}
	if token == "attacker-chosen-id" {
		t.Fatal("minted token must not be derived from an unsigned client-supplied header")
	}
	if !s.verify(token) {
		t.Fatal("minted token must verify against the same signer")
	}
}

func TestMintOrReadAnonymousInstanceID_ReusesValidToken(t *testing.T) {
	s := newAnonymousInstanceSigner("test-secret")
	r := httptest.NewRequest(http.MethodPost, "/issue-tmp-token", nil)
	minted := s.mintOrReadAnonymousInstanceID(httptest.NewRequest(http.MethodPost, "/issue-tmp-token", nil))
	r.Header.Set("X-Pipewave-ID", minted)

	token := s.mintOrReadAnonymousInstanceID(r)

	if token != minted {
		t.Fatalf("expected reuse of previously minted token %q, got %q", minted, token)
	}
}

func TestMintOrReadAnonymousInstanceID_MintsFreshWhenTokenForged(t *testing.T) {
	s := newAnonymousInstanceSigner("test-secret")
	other := newAnonymousInstanceSigner("different-secret")
	forged := other.mintOrReadAnonymousInstanceID(httptest.NewRequest(http.MethodPost, "/issue-tmp-token", nil))

	r := httptest.NewRequest(http.MethodPost, "/issue-tmp-token", nil)
	r.Header.Set("X-Pipewave-ID", forged)

	token := s.mintOrReadAnonymousInstanceID(r)

	if token == forged {
		t.Fatal("must not accept a token signed with a different secret")
	}
}

func TestReadAnonymousInstanceID_MissingHeader(t *testing.T) {
	s := newAnonymousInstanceSigner("test-secret")
	r := httptest.NewRequest(http.MethodGet, "/lp", nil)

	token, ok := s.readAnonymousInstanceID(r)

	if ok || token != "" {
		t.Fatalf("expected ok=false and empty token, got token=%q ok=%v", token, ok)
	}
}

func TestReadAnonymousInstanceID_ValidToken(t *testing.T) {
	s := newAnonymousInstanceSigner("test-secret")
	minted := s.mintOrReadAnonymousInstanceID(httptest.NewRequest(http.MethodPost, "/issue-tmp-token", nil))
	r := httptest.NewRequest(http.MethodGet, "/lp", nil)
	r.Header.Set("X-Pipewave-ID", minted)

	token, ok := s.readAnonymousInstanceID(r)

	if !ok || token != minted {
		t.Fatalf("expected ok=true token=%q, got token=%q ok=%v", minted, token, ok)
	}
}

func TestReadAnonymousInstanceID_RejectsTamperedPayload(t *testing.T) {
	s := newAnonymousInstanceSigner("test-secret")
	minted := s.mintOrReadAnonymousInstanceID(httptest.NewRequest(http.MethodPost, "/issue-tmp-token", nil))
	sepIdx := strings.LastIndex(minted, ".")
	tampered := "victim-instance-id" + minted[sepIdx:] // swap payload, keep original signature

	r := httptest.NewRequest(http.MethodGet, "/lp", nil)
	r.Header.Set("X-Pipewave-ID", tampered)

	_, ok := s.readAnonymousInstanceID(r)

	if ok {
		t.Fatal("expected tampered payload with mismatched signature to be rejected")
	}
}

func TestReadAnonymousInstanceID_RejectsExpiredToken(t *testing.T) {
	s := newAnonymousInstanceSigner("test-secret")
	expiredPayload := "some-id:" + strconv.FormatInt(time.Now().Add(-anonymousInstanceMaxAge-time.Hour).Unix(), 10)
	expiredToken := s.sign(expiredPayload)

	r := httptest.NewRequest(http.MethodGet, "/lp", nil)
	r.Header.Set("X-Pipewave-ID", expiredToken)

	_, ok := s.readAnonymousInstanceID(r)

	if ok {
		t.Fatal("expected a token minted beyond anonymousInstanceMaxAge to be rejected")
	}
}
