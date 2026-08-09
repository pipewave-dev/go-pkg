package wire

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type vector struct {
	Name  string `json:"name"`
	Frame struct {
		MsgType      string `json:"msgType"`
		ControlCode  *int   `json:"controlCode"`
		ID           string `json:"id"`
		ResponseToID string `json:"responseToId"`
		AckID        string `json:"ackId"`
		Error        string `json:"error"`
		BinaryHex    string `json:"binaryHex"`
	} `json:"frame"`
	Hex string `json:"hex"`
}

func loadVectors(t *testing.T) []vector {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "docs", "wire-vectors-v1.json")
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "vector file must exist at %s", path)
	var vs []vector
	require.NoError(t, json.Unmarshal(raw, &vs))
	require.NotEmpty(t, vs)
	return vs
}

func (v vector) toFrame(t *testing.T) *Frame {
	t.Helper()
	bin, err := hex.DecodeString(v.Frame.BinaryHex)
	require.NoError(t, err)
	f := &Frame{
		MsgType:      v.Frame.MsgType,
		ID:           v.Frame.ID,
		ResponseToID: v.Frame.ResponseToID,
		AckID:        v.Frame.AckID,
		Error:        v.Frame.Error,
		Binary:       bin,
	}
	if v.Frame.ControlCode != nil {
		f.ControlCode = byte(*v.Frame.ControlCode)
	}
	return f
}

func TestEncode_MatchesVectors(t *testing.T) {
	for _, v := range loadVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			got, err := Encode(v.toFrame(t))
			require.NoError(t, err)
			require.Equal(t, strings.ToLower(v.Hex), hex.EncodeToString(got))
		})
	}
}

func TestEncode_RejectsOverlongShortField(t *testing.T) {
	f := &Frame{MsgType: "t", ID: strings.Repeat("x", 256)}
	_, err := Encode(f)
	require.ErrorIs(t, err, ErrFieldTooLong)
}

func TestEncode_RejectsOverlongError(t *testing.T) {
	f := &Frame{MsgType: "t", Error: strings.Repeat("x", 65536)}
	_, err := Encode(f)
	require.ErrorIs(t, err, ErrFieldTooLong)
}
