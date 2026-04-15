package postgres

import (
	"testing"

	pgmigrations "github.com/pipewave-dev/go-pkg/provider/postgres/migrations"
)

func TestSelectPostgresMigrations(t *testing.T) {
	t.Parallel()

	migrations := []pgmigrations.Migration{
		{Version: 1, Name: "001_init.sql"},
		{Version: 2, Name: "002_add_index.sql"},
		{Version: 3, Name: "003_alter_table.sql"},
	}

	selected := selectPostgresMigrations(migrations, 1, 3)
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected migrations, got %d", len(selected))
	}
	if selected[0].Version != 2 || selected[1].Version != 3 {
		t.Fatalf("unexpected selected versions: %#v", selected)
	}
}
