package delivery

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"golang.org/x/time/rate"
)

func TestIPRateLimiter_AllowsWithinBurst(t *testing.T) {
	l := newIPRateLimiter(rate.Limit(1), 3)

	for i := 0; i < 3; i++ {
		if !l.Allow("1.2.3.4") {
			t.Fatalf("request %d expected to be allowed within burst", i)
		}
	}
}

func TestIPRateLimiter_BlocksOverBurst(t *testing.T) {
	l := newIPRateLimiter(rate.Limit(1), 2)

	l.Allow("1.2.3.4")
	l.Allow("1.2.3.4")
	if l.Allow("1.2.3.4") {
		t.Fatal("expected 3rd immediate request to be blocked once burst is exhausted")
	}
}

func TestIPRateLimiter_SeparateIPsIndependent(t *testing.T) {
	l := newIPRateLimiter(rate.Limit(1), 1)

	if !l.Allow("1.1.1.1") {
		t.Fatal("first IP should be allowed")
	}
	if !l.Allow("2.2.2.2") {
		t.Fatal("second, distinct IP must not be throttled by the first IP's bucket")
	}
}

func TestClientIP_FallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/issue-tmp-token", nil)
	r.RemoteAddr = "203.0.113.5:54321"

	ip := clientIP(r, "")

	if ip != "203.0.113.5" {
		t.Fatalf("expected 203.0.113.5, got %q", ip)
	}
}

func TestClientIP_UsesConfiguredHeader(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/issue-tmp-token", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 10.0.0.1")

	ip := clientIP(r, "X-Forwarded-For")

	if ip != "198.51.100.7" {
		t.Fatalf("expected first entry of X-Forwarded-For (198.51.100.7), got %q", ip)
	}
}

func TestClientIP_HeaderConfiguredButAbsentFallsBackToRemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/issue-tmp-token", nil)
	r.RemoteAddr = "203.0.113.9:8080"

	ip := clientIP(r, "X-Forwarded-For")

	if ip != "203.0.113.9" {
		t.Fatalf("expected fallback to RemoteAddr 203.0.113.9, got %q", ip)
	}
}
