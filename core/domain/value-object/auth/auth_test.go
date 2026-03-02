package voAuth

import (
	"testing"
)

// ---------------------------------------------------------------------------
// Roundtrip: Encode → Decode → Encode must produce identical bytes
// ---------------------------------------------------------------------------

func TestAuth_EncodeDecodeRoundtrip(t *testing.T) {
	tests := []struct {
		name string
		auth Auth
	}{
		{name: "nil (no auth)", auth: nil},
		{name: "UserAuth", auth: UserAuth("user-123", "instance-abc", false)},
		{name: "UserAuth admin", auth: UserAuth("admin-456", "instance-abc", true)},
		{name: "SystemAuth", auth: SystemAuth("pipewave")},
		{name: "AnonymousUserAuth", auth: AnonymousUserAuth("instance-xyz")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.auth == nil {
				// nil auth encodes to nil, no roundtrip needed
				if got := tt.auth.Encode(); got != nil {
					t.Errorf("nil.Encode() = %v, want nil", got)
				}
				return
			}

			encoded := tt.auth.Encode()
			if encoded == nil {
				t.Fatal("Encode() returned nil for non-nil auth")
			}

			decoded, err := Decode(encoded)
			if err != nil {
				t.Fatalf("Decode() error = %v", err)
			}

			// re-encode must match the original
			reEncoded := decoded.Encode()
			if string(encoded) != string(reEncoded) {
				t.Errorf("encode mismatch:\n  original  : %x\n  re-encoded: %x", encoded, reEncoded)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Encode deterministic: encoding the same auth multiple times must produce identical bytes
// ---------------------------------------------------------------------------

func TestAuth_Encode_Deterministic(t *testing.T) {
	tests := []struct {
		name string
		auth Auth
	}{
		{name: "UserAuth", auth: UserAuth("user-123", "instance-abc", false)},
		{name: "SystemAuth", auth: SystemAuth("pipewave")},
		{name: "AnonymousUserAuth", auth: AnonymousUserAuth("instance-xyz")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e1 := tt.auth.Encode()
			e2 := tt.auth.Encode()
			e3 := tt.auth.Encode()

			if string(e1) != string(e2) || string(e2) != string(e3) {
				t.Errorf("Encode() is not deterministic: %x / %x / %x", e1, e2, e3)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Decode with invalid input must return an error
// ---------------------------------------------------------------------------

func TestDecode_InvalidInput(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{name: "nil", input: nil},
		{name: "empty", input: []byte{}},
		{name: "invalid msgpack", input: []byte("not-valid-msgpack-bytes!!!")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Decode(tt.input); err == nil {
				t.Errorf("Decode(%q) expected error, got nil", tt.input)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// nil Auth must not panic when Encode is called
// ---------------------------------------------------------------------------

func TestAuth_Encode_Nil(t *testing.T) {
	var a Auth // a == nil
	if got := a.Encode(); got != nil {
		t.Errorf("nil.Encode() = %v, want nil", got)
	}
}

// ---------------------------------------------------------------------------
// Verify field values after decode
// ---------------------------------------------------------------------------

func TestDecode_UserAuth(t *testing.T) {
	original := UserAuth("user-123", "instance-abc", true)
	decoded, err := Decode(original.Encode())
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if decoded.user == nil {
		t.Fatal("decoded.user is nil")
	}
	if decoded.user.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", decoded.user.UserID, "user-123")
	}
	if decoded.user.InstanceID != "instance-abc" {
		t.Errorf("InstanceID = %q, want %q", decoded.user.InstanceID, "instance-abc")
	}
	if !decoded.user.isSystemAdmin {
		t.Error("isSystemAdmin = false, want true")
	}
	if decoded.user.isAnonymous {
		t.Error("isAnonymous = true, want false")
	}
}

func TestDecode_SystemAuth(t *testing.T) {
	original := SystemAuth("pipewave")
	decoded, err := Decode(original.Encode())
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if decoded.system == nil {
		t.Fatal("decoded.system is nil")
	}
	if decoded.system.SystemName != "pipewave" {
		t.Errorf("SystemName = %q, want %q", decoded.system.SystemName, "pipewave")
	}
	if decoded.user != nil {
		t.Error("decoded.user should be nil for system auth")
	}
}

func TestDecode_AnonymousUserAuth(t *testing.T) {
	original := AnonymousUserAuth("instance-xyz")
	decoded, err := Decode(original.Encode())
	if err != nil {
		t.Fatalf("Decode error: %v", err)
	}
	if decoded.user == nil {
		t.Fatal("decoded.user is nil")
	}
	if !decoded.user.isAnonymous {
		t.Error("isAnonymous = false, want true")
	}
	if decoded.user.InstanceID != "instance-xyz" {
		t.Errorf("InstanceID = %q, want %q", decoded.user.InstanceID, "instance-xyz")
	}
}
