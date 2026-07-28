package metrics_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"syscall"
	"testing"

	"github.com/pipewave-dev/go-pkg/pkg/metrics"
	"github.com/stretchr/testify/require"
)

func TestSanitizeMsgType(t *testing.T) {
	allow := metrics.BuildAllowlist([]string{"CHAT", "NEWS"})

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"heartbeat system byte", string([]byte{202}), metrics.MsgTypeHeartbeat},
		{"ack system byte", string([]byte{203}), metrics.MsgTypeAck},
		{"allowlisted", "CHAT", "CHAT"},
		{"allowlisted second", "NEWS", "NEWS"},
		{"not allowlisted", "SECRET_TYPE", metrics.MsgTypeOther},
		{"empty", "", metrics.MsgTypeOther},
		{"non printable not system", string([]byte{7}), metrics.MsgTypeOther},
		{"too long even if allowlisted-looking", strings.Repeat("A", 33), metrics.MsgTypeOther},
		{"exactly 32 but not allowlisted", strings.Repeat("A", 32), metrics.MsgTypeOther},
		{"embedded newline", "CH\nAT", metrics.MsgTypeOther},
		{"unicode", "chào", metrics.MsgTypeOther},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, metrics.SanitizeMsgType(tt.raw, allow))
		})
	}
}

func TestSanitizeMsgType_EmptyAllowlistCollapsesAppTypes(t *testing.T) {
	allow := metrics.BuildAllowlist(nil)
	require.Equal(t, metrics.MsgTypeOther, metrics.SanitizeMsgType("CHAT", allow))
	// system types still resolve without an allowlist
	require.Equal(t, metrics.MsgTypeHeartbeat, metrics.SanitizeMsgType(string([]byte{202}), allow))
}

func TestClassifyCallbackError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
		want   string
	}{
		{"nil error 200", nil, 200, ""},
		{"deadline exceeded", context.DeadlineExceeded, 0, "timeout"},
		{"wrapped deadline", fmt.Errorf("post failed: %w", context.DeadlineExceeded), 0, "timeout"},
		{"conn refused", fmt.Errorf("dial: %w", syscall.ECONNREFUSED), 0, "conn_refused"},
		{"dns error", &net.DNSError{UnwrapErr: nil, Err: "no such host", Name: "", Server: "", IsTimeout: false, IsTemporary: false, IsNotFound: false}, 0, "dns"},
		{"wrapped dns", fmt.Errorf("lookup: %w", &net.DNSError{UnwrapErr: nil, Err: "nope", Name: "", Server: "", IsTimeout: false, IsTemporary: false, IsNotFound: false}), 0, "dns"},
		{"status 500", nil, 500, "status_5xx"},
		{"status 503", nil, 503, "status_5xx"},
		{"status 404", nil, 404, "status_4xx"},
		{"status 400", nil, 400, "status_4xx"},
		{"unknown error", errors.New("boom"), 0, "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, metrics.ClassifyCallbackError(tt.err, tt.status))
		})
	}
}

func TestClassifyCallbackError_ErrorWinsOverStatus(t *testing.T) {
	// transport error with a zero status must not be read as status_4xx
	require.Equal(t, "timeout", metrics.ClassifyCallbackError(context.DeadlineExceeded, 0))
}
