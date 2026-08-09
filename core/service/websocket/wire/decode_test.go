package wire

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecode_MatchesVectors(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(strings.ToLower(v.Hex))
			require.NoError(t, err)

			got, err := Decode(raw)
			require.NoError(t, err)

			want := v.toFrame(t)
			require.Equal(t, want.MsgType, got.MsgType)
			require.Equal(t, want.ControlCode, got.ControlCode)
			require.Equal(t, want.ID, got.ID)
			require.Equal(t, want.ResponseToID, got.ResponseToID)
			require.Equal(t, want.AckID, got.AckID)
			require.Equal(t, want.Error, got.Error)
			require.Equal(t, want.Binary, got.Binary)
		})
	}
}

func TestDecode_RoundTrip(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			orig := v.toFrame(t)
			encoded, err := Encode(orig)
			require.NoError(t, err)
			back, err := Decode(encoded)
			require.NoError(t, err)
			reencoded, err := Encode(back)
			require.NoError(t, err)
			require.Equal(t, encoded, reencoded)
		})
	}
}

func TestDecode_RejectsMalformed(t *testing.T) {
	cases := map[string][]byte{
		"empty":                {},
		"one_byte":             {0x01},
		"unknown_version":      {0x09, 0x00, 0xCA},
		"control_missing_code": {0x01, 0x00},
		"msgtype_truncated":    {0x01, 0x05, 0x68},
		"id_len_truncated":     {0x11, 0x01, 0x74},
		"id_body_truncated":    {0x11, 0x01, 0x74, 0x05, 0x61},
		"error_len_truncated":  {0x81, 0x01, 0x74, 0x00},
		"error_body_truncated": {0x81, 0x01, 0x74, 0x00, 0x05, 0x61},
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Decode(raw)
			require.Error(t, err, "must reject, not accept")
		})
	}
}

func TestDecode_DoesNotCopyBinary(t *testing.T) {
	// msgType "t", binary "ab"
	raw := []byte{0x01, 0x01, 0x74, 0x61, 0x62}
	f, err := Decode(raw)
	require.NoError(t, err)
	require.Equal(t, []byte{0x61, 0x62}, f.Binary)
	raw[3] = 0x7A // mutate backing array
	require.Equal(t, byte(0x7A), f.Binary[0], "Binary must alias input, not copy")
}
