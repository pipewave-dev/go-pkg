package migrations

import "testing"

func TestLatestVersion(t *testing.T) {
	t.Parallel()

	version, err := LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion returned error: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected latest version 1, got %d", version)
	}
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	version, err := parseVersion("001_init.sql")
	if err != nil {
		t.Fatalf("parseVersion returned error: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
	}
}
