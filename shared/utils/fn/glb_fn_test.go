package fn

import (
	"testing"

	"github.com/google/uuid"
)

func TestUuidToString_and_ScanUUID_roundtrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		id   uuid.UUID
	}{
		{
			name: "nil uuid",
			id:   uuid.Nil,
		},
		{
			name: "max uuid",
			id:   uuid.UUID{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
		},
		{
			name: "known uuid",
			id:   uuid.MustParse("550e8400-e29b-41d4-a716-446655440000"),
		},
		{
			name: "random v7 uuid",
			id:   NewUUID(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			encoded := UuidToString(tt.id)
			if encoded == "" && tt.id != uuid.Nil {
				t.Fatal("UuidToString returned empty string for non-nil uuid")
			}

			decoded, err := ScanUUID(encoded)
			if err != nil {
				t.Fatalf("ScanUUID returned error: %v", err)
			}

			if decoded != tt.id {
				t.Errorf("roundtrip mismatch: got %s, want %s", decoded, tt.id)
			}
		})
	}
}

func TestUuidToString(t *testing.T) {
	t.Parallel()

	t.Run("nil uuid encodes to 0", func(t *testing.T) {
		t.Parallel()
		result := UuidToString(uuid.Nil)
		if result != "0" {
			t.Errorf("expected '0', got %q", result)
		}
	})

	t.Run("different uuids produce different strings", func(t *testing.T) {
		t.Parallel()
		id1 := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		id2 := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

		s1 := UuidToString(id1)
		s2 := UuidToString(id2)

		if s1 == s2 {
			t.Errorf("different uuids produced same string: %s", s1)
		}
	})

	t.Run("output is shorter than standard uuid string", func(t *testing.T) {
		t.Parallel()
		id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
		encoded := UuidToString(id)
		standard := id.String() // 36 chars with hyphens

		if len(encoded) >= len(standard) {
			t.Errorf("base62 encoding (%d chars) should be shorter than standard (%d chars)", len(encoded), len(standard))
		}
	})
}

func TestScanUUID(t *testing.T) {
	t.Parallel()

	t.Run("invalid base62 string returns error", func(t *testing.T) {
		t.Parallel()
		_, err := ScanUUID("!@#$%")
		if err == nil {
			t.Fatal("expected error for invalid base62 string")
		}
	})

	t.Run("value too large for uuid returns error", func(t *testing.T) {
		t.Parallel()
		// 17 bytes worth of base62 — larger than 16-byte uuid
		_, err := ScanUUID("zzzzzzzzzzzzzzzzzzzzzzzzzzz")
		if err == nil {
			t.Fatal("expected error for oversized value")
		}
	})

	t.Run("empty string returns error", func(t *testing.T) {
		t.Parallel()
		_, err := ScanUUID("")
		if err == nil {
			t.Fatal("expected error for empty string")
		}
	})

	t.Run("0 decodes to nil uuid", func(t *testing.T) {
		t.Parallel()
		id, err := ScanUUID("0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id != uuid.Nil {
			t.Errorf("expected nil uuid, got %s", id)
		}
	})
}
