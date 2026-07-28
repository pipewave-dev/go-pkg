package types_test

import (
	"testing"

	"github.com/pipewave-dev/go-pkg/export/types"
	"github.com/stretchr/testify/require"
)

func TestMetricsT_Defaults(t *testing.T) {
	m := &types.MetricsT{}
	m.LoadDefaultForTest()
	require.Equal(t, 9090, m.Port)
	require.Equal(t, "/metrics", m.Path)
	require.False(t, m.Enabled)
}

func TestMetricsT_ValidateDisabledSkipsChecks(t *testing.T) {
	m := &types.MetricsT{Enabled: false, Port: -1, Path: "nope"}
	require.NotPanics(t, m.ValidateForTest)
}

func TestMetricsT_ValidateEnabled(t *testing.T) {
	tests := []struct {
		name      string
		m         types.MetricsT
		wantPanic bool
	}{
		{"valid", types.MetricsT{Enabled: true, Port: 9090, Path: "/metrics"}, false},
		{"port zero", types.MetricsT{Enabled: true, Port: 0, Path: "/metrics"}, true},
		{"port too high", types.MetricsT{Enabled: true, Port: 70000, Path: "/metrics"}, true},
		{"path missing slash", types.MetricsT{Enabled: true, Port: 9090, Path: "metrics"}, true},
		{"allowlist too long", types.MetricsT{
			Enabled: true, Port: 9090, Path: "/metrics",
			MsgTypeAllowlist: []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}, // 33
		}, true},
		{"allowlist non printable", types.MetricsT{
			Enabled: true, Port: 9090, Path: "/metrics",
			MsgTypeAllowlist: []string{string([]byte{7})},
		}, true},
		{"allowlist ok", types.MetricsT{
			Enabled: true, Port: 9090, Path: "/metrics",
			MsgTypeAllowlist: []string{"CHAT", "NEWS"},
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.m
			if tt.wantPanic {
				require.Panics(t, m.ValidateForTest)
				return
			}
			require.NotPanics(t, m.ValidateForTest)
		})
	}
}
