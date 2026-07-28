// Package metrics creates OTEL metric instruments for pipewave.
//
// It reads the global MeterProvider via otel.GetMeterProvider() and never
// sets it: the process that embeds pipewave owns that decision. When no
// provider is configured the OTEL API returns no-op instruments, so every
// Record* call is a cheap no-op and pipewave adds no metrics overhead.
package metrics

import (
	"context"
	"errors"
	"net"
	"net/http"
	"syscall"
)

// Label values for the msg_type dimension.
const (
	MsgTypeOther     = "other"
	MsgTypeHeartbeat = "heartbeat"
	MsgTypeAck       = "ack"
)

// System message types are single non-printable bytes on the wire
// (core/service/websocket/0.message_type.go). Map them to readable labels.
const (
	sysByteHeartbeat = 202
	sysByteAck       = 203
)

// maxMsgTypeLen bounds label length so a hostile client cannot inflate
// Prometheus memory with long label values.
const maxMsgTypeLen = 32

// BuildAllowlist converts configured msg_type entries into a lookup set.
// A nil or empty slice yields an empty set, which collapses every
// app-level msg_type to MsgTypeOther.
func BuildAllowlist(entries []string) map[string]struct{} {
	set := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		set[e] = struct{}{}
	}
	return set
}

// SanitizeMsgType maps a wire msg_type to a bounded, readable label value.
//
// msg_type is client-controlled, so it is never used as a label verbatim:
// unknown values collapse to MsgTypeOther. This keeps cardinality bounded by
// len(allowlist)+3 regardless of what clients send.
func SanitizeMsgType(raw string, allowlist map[string]struct{}) string {
	if len(raw) == 1 {
		switch raw[0] {
		case sysByteHeartbeat:
			return MsgTypeHeartbeat
		case sysByteAck:
			return MsgTypeAck
		}
	}
	if raw == "" || len(raw) > maxMsgTypeLen {
		return MsgTypeOther
	}
	if _, ok := allowlist[raw]; !ok {
		return MsgTypeOther
	}
	if !isPrintableASCII(raw) {
		return MsgTypeOther
	}
	return raw
}

// isPrintableASCII reports whether s consists solely of printable ASCII
// (0x20-0x7E). Anything else would render as garbage in a dashboard.
func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7E {
			return false
		}
	}
	return true
}

// Label values for the callback reason dimension.
const (
	ReasonTimeout     = "timeout"
	ReasonConnRefused = "conn_refused"
	ReasonDNS         = "dns"
	ReasonStatus4xx   = "status_4xx"
	ReasonStatus5xx   = "status_5xx"
	ReasonOther       = "other"
)

// ClassifyCallbackError reduces a callback failure to a bounded reason label.
// Returns "" when the call succeeded (nil err and a non-error status).
//
// Transport errors are classified before status codes: a timeout carries
// statusCode 0, which must not be misread as a 4xx.
func ClassifyCallbackError(err error, statusCode int) string {
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return ReasonTimeout
		case errors.Is(err, syscall.ECONNREFUSED):
			return ReasonConnRefused
		}
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			return ReasonDNS
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			return ReasonTimeout
		}
		return ReasonOther
	}
	switch {
	case statusCode >= http.StatusInternalServerError:
		return ReasonStatus5xx
	case statusCode >= http.StatusBadRequest:
		return ReasonStatus4xx
	}
	return ""
}
