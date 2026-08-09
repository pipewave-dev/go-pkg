package wire

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
)

// updateMalformed regenerates the committed cross-language expectations from
// the current decoder. Deliberately opt-in: a drift failure should be read and
// understood, not silenced by rerunning with -update out of habit.
var updateMalformed = flag.Bool("update", false,
	"rewrite docs/wire-malformed-cases-v1.json from the current decoder")

// Differential malformed-input harness.
//
// The frozen conformance vectors cover only WELL-FORMED frames. That gap let a
// real bug through every per-language suite: Dart threw on invalid UTF-8 where
// Go and TypeScript accepted it, so a client could craft a frame that Go/TS
// clients processed and every Dart client silently discarded. Reject-vs-accept
// alone would not have caught it — the frame was accepted STRUCTURALLY by all
// three; they differed on CONTENT.
//
// This file is the source of truth for that harness. Go's behaviour is recorded
// into docs/wire-malformed-cases-v1.json, which the TypeScript and Dart suites
// consume. Run with -update after any intentional decoder change:
//
//	go test ./core/service/websocket/wire/ -run TestMalformedCases -update
//
// then copy the regenerated file into the other two repos (see Makefile target
// or docs/wire-protocol-v1.md).

type malformedCase struct {
	Name     string `json:"name"`
	Hex      string `json:"hex"`
	Rejected bool   `json:"rejected"`

	// Decoded string fields are recorded ONLY as raw UTF-8 bytes (hex). Go
	// RETAINS invalid bytes verbatim while TS and Dart SUBSTITUTE U+FFFD; a
	// string-level comparison silently normalises that away and would hide the
	// very class of bug this harness exists to catch.
	//
	// The decoded strings are deliberately NOT serialised as JSON strings:
	// encoding/json replaces invalid UTF-8 with U+FFFD on the way out, so such
	// a field would not even survive its own round trip.
	MsgTypeHex      string `json:"msgTypeHex"`
	ControlCode     int    `json:"controlCode"`
	IDHex           string `json:"idHex"`
	ResponseToIDHex string `json:"responseToIdHex"`
	AckIDHex        string `json:"ackIdHex"`
	ErrorHex        string `json:"errorHex"`
	BinaryHex       string `json:"binaryHex"`

	// UTF8Lossy marks cases where Go's retained bytes are not valid UTF-8, so
	// TS and Dart are expected to differ by substituting U+FFFD. For these the
	// cross-language assertion is "TS and Dart agree with each other and do not
	// throw", not "all three agree" — the spec permits either behaviour.
	UTF8Lossy bool   `json:"utf8Lossy"`
	Note      string `json:"note"`
}

// malformedInputs are hand-written hostile frames. Add to this list; never
// hand-edit the generated JSON.
var malformedInputs = []struct {
	name  string
	bytes []byte
	note  string
}{
	// Structural truncation: all three implementations must reject.
	{"empty", []byte{}, "zero-length frame"},
	{"one_byte", []byte{0x01}, "shorter than the 2-byte minimum"},
	{"unknown_version_9", []byte{0x09, 0x00, 0xCA}, "version nibble 9 is not v1"},
	{"unknown_version_0", []byte{0x00, 0x01, 0x74}, "version nibble 0 is not v1"},
	{"msgpack_v0_frame", []byte{0x83, 0xA1, 0x74, 0xA1, 0x6D}, "a real msgpack map header: the stale-client cutover case"},
	{"control_missing_code", []byte{0x01, 0x00}, "control frame with no code byte"},
	{"msgtype_truncated", []byte{0x01, 0x05, 0x68}, "msgType claims 5 bytes, 1 present"},
	{"msgtype_len_255_absent", []byte{0x01, 0xFF}, "msgType claims 255 bytes, none present"},
	{"id_len_truncated", []byte{0x11, 0x01, 0x74}, "id flag set, no length byte"},
	{"id_body_truncated", []byte{0x11, 0x01, 0x74, 0x05, 0x61}, "id claims 5 bytes, 1 present"},
	{"error_len_truncated", []byte{0x81, 0x01, 0x74, 0x00}, "error flag set, only 1 of 2 length bytes"},
	{"error_body_truncated", []byte{0x81, 0x01, 0x74, 0x00, 0x05, 0x61}, "error claims 5 bytes, 1 present"},
	{"error_len_max_absent", []byte{0x81, 0x01, 0x74, 0xFF, 0xFF}, "error claims 65535 bytes, none present: allocation-bomb shape"},
	{"all_flags_no_payload", []byte{0xF1, 0x01, 0x74}, "every optional flag set, no field bytes follow"},
	{"ackid_body_truncated", []byte{0x41, 0x01, 0x74, 0x09, 0x61}, "ackId claims 9 bytes, 1 present"},
	{"responseto_body_truncated", []byte{0x21, 0x01, 0x74, 0x09, 0x61}, "responseToId claims 9 bytes, 1 present"},

	// Structurally valid but hostile CONTENT: behaviour must match.
	{"invalid_utf8_msgtype", []byte{0x01, 0x02, 0xFF, 0xFE}, "invalid UTF-8 in msgType: the bug D shape"},
	{"invalid_utf8_error", []byte{0x81, 0x01, 0x74, 0x00, 0x02, 0xFF, 0xFE}, "invalid UTF-8 in the error field"},
	{"invalid_utf8_id", []byte{0x11, 0x01, 0x74, 0x02, 0xC3, 0x28}, "overlong/invalid UTF-8 sequence in id"},
	{"truncated_utf8_msgtype", []byte{0x01, 0x02, 0xE4, 0xB8}, "first 2 bytes of a 3-byte CJK char"},
	{"lone_surrogate_bytes", []byte{0x01, 0x03, 0xED, 0xA0, 0x80}, "CESU-8 encoded lone surrogate D800"},
	{"nul_in_msgtype", []byte{0x01, 0x03, 0x61, 0x00, 0x62}, "embedded NUL byte in msgType"},

	// Non-canonical: must be accepted, decoding to empty.
	{"noncanon_id_len0", []byte{0x11, 0x01, 0x74, 0x00}, "id flag set, declared length 0"},
	{"noncanon_error_len0", []byte{0x81, 0x01, 0x74, 0x00, 0x00}, "error flag set, declared length 0"},
	{"noncanon_all_len0", []byte{0xF1, 0x01, 0x74, 0x00, 0x00, 0x00, 0x00, 0x00}, "every optional flag set, all declared length 0"},

	// Unknown control codes: must be surfaced, not rejected.
	{"unknown_control_00", []byte{0x01, 0x00, 0x00}, "control code 0x00 is undefined in v1"},
	{"unknown_control_99", []byte{0x01, 0x00, 0x99}, "control code 0x99 is undefined in v1"},
	{"unknown_control_ff", []byte{0x01, 0x00, 0xFF}, "control code 0xFF is undefined in v1"},
	{"unknown_control_with_body", []byte{0x01, 0x00, 0x77, 0x61, 0x62}, "undefined control code carrying a binary body"},

	// Degenerate but legal.
	{"bare_minimum", []byte{0x01, 0x00, 0xCA}, "smallest legal frame: a heartbeat"},
	{"empty_msgtype_via_control", []byte{0x01, 0x00, 0xCB}, "ack control frame, no body"},
	{"high_bytes_binary", []byte{0x01, 0x01, 0x74, 0x00, 0xFF, 0xCA, 0xCB}, "binary payload containing control-code bytes"},
}

func buildMalformedCases() []malformedCase {
	out := make([]malformedCase, 0, len(malformedInputs))
	for _, in := range malformedInputs {
		c := malformedCase{
			Name: in.name,
			Hex:  hex.EncodeToString(in.bytes),
			Note: in.note,
		}
		f, err := Decode(in.bytes)
		if err != nil {
			c.Rejected = true
		} else {
			c.MsgTypeHex = hex.EncodeToString([]byte(f.MsgType))
			c.ControlCode = int(f.ControlCode)
			c.IDHex = hex.EncodeToString([]byte(f.ID))
			c.ResponseToIDHex = hex.EncodeToString([]byte(f.ResponseToID))
			c.AckIDHex = hex.EncodeToString([]byte(f.AckID))
			c.ErrorHex = hex.EncodeToString([]byte(f.Error))
			c.BinaryHex = hex.EncodeToString(f.Binary)
			c.UTF8Lossy = !utf8.ValidString(f.MsgType) ||
				!utf8.ValidString(f.ID) ||
				!utf8.ValidString(f.ResponseToID) ||
				!utf8.ValidString(f.AckID) ||
				!utf8.ValidString(f.Error)
		}
		out = append(out, c)
	}
	return out
}

func malformedCasesPath() string {
	return filepath.Join("..", "..", "..", "..", "docs", "wire-malformed-cases-v1.json")
}

// TestMalformedCases fails when the decoder's behaviour on hostile input drifts
// from the committed cross-language expectations. A failure here means either a
// real regression, or an intentional change that must be propagated to the
// TypeScript and Dart suites — never something to silence.
func TestMalformedCases(t *testing.T) {
	got := buildMalformedCases()

	if *updateMalformed {
		blob, err := json.MarshalIndent(got, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(malformedCasesPath(), append(blob, '\n'), 0o644))
		t.Logf("wrote %d cases to %s — now copy it into the TS and Dart repos",
			len(got), malformedCasesPath())
		return
	}

	raw, err := os.ReadFile(malformedCasesPath())
	require.NoError(t, err, "run with -update to generate")
	var want []malformedCase
	require.NoError(t, json.Unmarshal(raw, &want))

	require.Equal(t, len(want), len(got),
		"case count changed; regenerate with -update and propagate to TS and Dart")
	for i := range want {
		require.Equal(t, want[i], got[i],
			"decoder behaviour drifted for case %q", want[i].Name)
	}
}

// TestMalformedCases_Coverage guards the harness itself. A suite that only
// covered rejections would not have caught the invalid-UTF-8 bug, because that
// frame was accepted by every implementation — they differed on the decoded
// content. These assertions fail if someone prunes the interesting cases away.
func TestMalformedCases_Coverage(t *testing.T) {
	cases := buildMalformedCases()

	var rejected, accepted, lossy int
	byName := make(map[string]malformedCase, len(cases))
	for _, c := range cases {
		byName[c.Name] = c
		switch {
		case c.Rejected:
			rejected++
		default:
			accepted++
			if c.UTF8Lossy {
				lossy++
			}
		}
	}

	require.Greater(t, rejected, 0, "harness must cover rejected frames")
	require.Greater(t, accepted, 0, "harness must cover ACCEPTED hostile frames")
	require.Greater(t, lossy, 0,
		"harness must cover frames where Go retains invalid UTF-8 — the bug-D class")

	// The specific frame that regressed. If this is ever rejected by Go, the
	// cross-language expectation has changed and every client needs review.
	bugD, ok := byName["invalid_utf8_msgtype"]
	require.True(t, ok, "the bug-D regression case must not be removed")
	require.False(t, bugD.Rejected,
		"Go must accept invalid UTF-8 in msgType; TS and Dart match this by substituting U+FFFD")
	require.Equal(t, "fffe", bugD.MsgTypeHex, "Go must retain the raw invalid bytes")

	// A stale msgpack frame must always fail closed — this is the cutover story.
	stale, ok := byName["msgpack_v0_frame"]
	require.True(t, ok)
	require.True(t, stale.Rejected,
		"a pre-v1 msgpack frame must be rejected, never misparsed")
}
