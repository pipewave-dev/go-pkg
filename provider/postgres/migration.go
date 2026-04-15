package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	pgmigrations "github.com/pipewave-dev/go-pkg/provider/postgres/migrations"
)

const (
	// postgresSchemaVersionTable = "schema_version"
	postgresSchemaVersionRowID = 1
)

func LatestMigrationVersion() (int, error) {
	return pgmigrations.LatestVersion()
}

func HandleStartupMigration(ctx context.Context, pool *pgxpool.Pool, autoMigration bool) error {
	if autoMigration {
		return RunMigration(ctx, pool)
	}
	return VerifyMigration(ctx, pool)
}

func RunMigration(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := pgmigrations.All()
	if err != nil {
		return err
	}

	latestVersion := 0
	if len(migrations) > 0 {
		latestVersion = migrations[len(migrations)-1].Version
	}

	if err = ensureSchemaVersionTable(ctx, pool); err != nil {
		return err
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("postgres: failed to begin migration transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(ctx)
		}
	}()

	inserted, err := bootstrapSchemaVersion(ctx, tx, latestVersion)
	if err != nil {
		return err
	}
	if inserted {
		if err = runPostgresMigrations(ctx, tx, migrations); err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("postgres: failed to commit bootstrap migration transaction: %w", err)
		}
		committed = true
		return nil
	}

	currentVersion, err := getSchemaVersionForUpdate(ctx, tx)
	if err != nil {
		return err
	}
	if currentVersion > latestVersion {
		return fmt.Errorf("postgres: database schema version %d is newer than application version %d", currentVersion, latestVersion)
	}
	if currentVersion == latestVersion {
		if err = tx.Commit(ctx); err != nil {
			return fmt.Errorf("postgres: failed to commit no-op migration transaction: %w", err)
		}
		committed = true
		return nil
	}

	tag, err := tx.Exec(
		ctx,
		`UPDATE schema_version
		 SET version = $1, updated_at = NOW()
		 WHERE id = $2 AND version = $3`,
		latestVersion,
		postgresSchemaVersionRowID,
		currentVersion,
	)
	if err != nil {
		return fmt.Errorf("postgres: failed to advance schema version from %d to %d: %w", currentVersion, latestVersion, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("postgres: failed to advance schema version from %d to %d: expected 1 updated row, got %d", currentVersion, latestVersion, tag.RowsAffected())
	}

	if err = runPostgresMigrations(ctx, tx, selectPostgresMigrations(migrations, currentVersion, latestVersion)); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: failed to commit migration transaction: %w", err)
	}
	committed = true
	return nil
}

func VerifyMigration(ctx context.Context, pool *pgxpool.Pool) error {
	latestVersion, err := LatestMigrationVersion()
	if err != nil {
		return err
	}
	if err = ensureSchemaVersionTable(ctx, pool); err != nil {
		return err
	}

	currentVersion, found, err := getSchemaVersion(ctx, pool)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("postgres: migration has not been run yet: schema_version row is missing, latest version is %d", latestVersion)
	}
	if currentVersion != latestVersion {
		if currentVersion > latestVersion {
			return fmt.Errorf("postgres: database schema version %d is newer than application version %d", currentVersion, latestVersion)
		}
		return fmt.Errorf("postgres: migration has not been run yet: current version is %d, latest version is %d", currentVersion, latestVersion)
	}
	return nil
}

func ensureSchemaVersionTable(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_version (
			id SMALLINT PRIMARY KEY,
			version INTEGER NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("postgres: failed to ensure schema_version table: %w", err)
	}
	return nil
}

func bootstrapSchemaVersion(ctx context.Context, tx pgx.Tx, latestVersion int) (bool, error) {
	var insertedID int
	err := tx.QueryRow(
		ctx,
		`INSERT INTO schema_version (id, version, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT DO NOTHING
		 RETURNING id`,
		postgresSchemaVersionRowID,
		latestVersion,
	).Scan(&insertedID)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return false, fmt.Errorf("postgres: failed to bootstrap schema_version row: %w", err)
}

func getSchemaVersionForUpdate(ctx context.Context, tx pgx.Tx) (int, error) {
	var version int
	err := tx.QueryRow(
		ctx,
		`SELECT version
		 FROM schema_version
		 WHERE id = $1
		 FOR UPDATE`,
		postgresSchemaVersionRowID,
	).Scan(&version)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("postgres: schema_version row is missing")
		}
		return 0, fmt.Errorf("postgres: failed to read schema version for update: %w", err)
	}
	return version, nil
}

func getSchemaVersion(ctx context.Context, pool *pgxpool.Pool) (version int, found bool, err error) {
	err = pool.QueryRow(
		ctx,
		`SELECT version
		 FROM schema_version
		 WHERE id = $1`,
		postgresSchemaVersionRowID,
	).Scan(&version)
	if err == nil {
		return version, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return 0, false, fmt.Errorf("postgres: failed to read schema version: %w", err)
}

func runPostgresMigrations(ctx context.Context, tx pgx.Tx, migrations []pgmigrations.Migration) error {
	for _, migration := range migrations {
		if _, err := tx.Exec(ctx, migration.SQL); err != nil {
			return fmt.Errorf("postgres: failed to run migration %s: %w", migration.Name, err)
		}
	}
	return nil
}

func selectPostgresMigrations(migrations []pgmigrations.Migration, currentVersion, latestVersion int) []pgmigrations.Migration {
	selected := make([]pgmigrations.Migration, 0, len(migrations))
	for _, migration := range migrations {
		if migration.Version > currentVersion && migration.Version <= latestVersion {
			selected = append(selected, migration)
		}
	}
	return selected
}
