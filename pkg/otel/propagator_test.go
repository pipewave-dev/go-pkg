package otel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
)

func TestCusMapCarrier_Encode(t *testing.T) {
	tests := []struct {
		name      string
		carrier   cusMapCarrier
		wantNil   bool
		wantPanic bool
	}{
		{
			name: "encode non-empty carrier",
			carrier: cusMapCarrier{
				MapCarrier: propagation.MapCarrier{
					"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
					"tracestate":  "congo=t61rcWkgMzE",
				},
			},
			wantNil:   false,
			wantPanic: false,
		},
		{
			name: "encode empty carrier",
			carrier: cusMapCarrier{
				MapCarrier: propagation.MapCarrier{},
			},
			wantNil:   true,
			wantPanic: false,
		},
		{
			name: "encode nil carrier",
			carrier: cusMapCarrier{
				MapCarrier: nil,
			},
			wantNil:   true,
			wantPanic: false,
		},
		{
			name: "encode carrier with single key",
			carrier: cusMapCarrier{
				MapCarrier: propagation.MapCarrier{
					"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
				},
			},
			wantNil:   false,
			wantPanic: false,
		},
		{
			name: "encode carrier with special characters",
			carrier: cusMapCarrier{
				MapCarrier: propagation.MapCarrier{
					"key": "value with spaces and 特殊字符",
				},
			},
			wantNil:   false,
			wantPanic: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.wantPanic {
				assert.Panics(t, func() {
					tt.carrier.Encode()
				})
				return
			}

			result := tt.carrier.Encode()

			if tt.wantNil {
				assert.Nil(t, result)
			} else {
				assert.NotNil(t, result)
				assert.Greater(t, len(result), 0)
			}
		})
	}
}

func TestCusMapCarrier_Decode(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		wantLen int
	}{
		{
			name:    "decode empty data",
			data:    []byte{},
			wantErr: false,
			wantLen: 0,
		},
		{
			name:    "decode nil data",
			data:    nil,
			wantErr: false,
			wantLen: 0,
		},
		{
			name:    "decode invalid msgpack data",
			data:    []byte{0xFF, 0xFF, 0xFF},
			wantErr: true,
			wantLen: 0,
		},
		{
			name: "decode valid msgpack data",
			data: func() []byte {
				carrier := cusMapCarrier{
					MapCarrier: propagation.MapCarrier{
						"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
						"tracestate":  "congo=t61rcWkgMzE",
					},
				}
				return carrier.Encode()
			}(),
			wantErr: false,
			wantLen: 2,
		},
		{
			name: "decode single entry",
			data: func() []byte {
				carrier := cusMapCarrier{
					MapCarrier: propagation.MapCarrier{
						"key": "value",
					},
				}
				return carrier.Encode()
			}(),
			wantErr: false,
			wantLen: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cusMapCarrier{}
			err := c.Decode(tt.data)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantLen, len(c.MapCarrier))
			}
		})
	}
}

func TestCusMapCarrier_EncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		original propagation.MapCarrier
	}{
		{
			name: "round trip with trace data",
			original: propagation.MapCarrier{
				"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
				"tracestate":  "congo=t61rcWkgMzE",
			},
		},
		{
			name: "round trip with multiple keys",
			original: propagation.MapCarrier{
				"key1": "value1",
				"key2": "value2",
				"key3": "value3",
			},
		},
		{
			name: "round trip with unicode",
			original: propagation.MapCarrier{
				"unicode": "こんにちは 世界 🌍",
			},
		},
		{
			name:     "round trip empty map",
			original: propagation.MapCarrier{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			carrier := cusMapCarrier{MapCarrier: tt.original}
			encoded := carrier.Encode()

			// Decode
			decoded := &cusMapCarrier{}
			err := decoded.Decode(encoded)
			require.NoError(t, err)

			// Verify
			if len(tt.original) == 0 {
				assert.Nil(t, encoded)
				assert.Equal(t, 0, len(decoded.MapCarrier))
			} else {
				assert.Equal(t, tt.original, decoded.MapCarrier)
			}
		})
	}
}
