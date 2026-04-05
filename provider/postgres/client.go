package postgres

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
)

//go:embed migrations/001_init.sql
var migrationSQL string

func New(cfg configprovider.ConfigStore) *pgxpool.Pool {
	pgCfg := cfg.Env().Postgres

	dsn := fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s sslmode=%s",
		pgCfg.Host, pgCfg.Port, pgCfg.DBName, pgCfg.User, pgCfg.Password, pgCfg.SSLMode,
	)

	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		panic(fmt.Sprintf("postgres: failed to parse config: %v", err))
	}

	if pgCfg.MaxConns > 0 {
		poolCfg.MaxConns = pgCfg.MaxConns
	}
	if pgCfg.MinConns > 0 {
		poolCfg.MinConns = pgCfg.MinConns
	}

	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		panic(fmt.Sprintf("postgres: failed to create pool: %v", err))
	}

	if err := pool.Ping(context.Background()); err != nil {
		panic(fmt.Sprintf("postgres: failed to ping: %v", err))
	}

	if cfg.Env().AutoMigration && pgCfg.CreateTables {
		createTables(pool)
	}

	return pool
}

func createTables(pool *pgxpool.Pool) {
	_, err := pool.Exec(context.Background(), migrationSQL)
	if err != nil {
		panic(fmt.Sprintf("postgres: failed to run migration: %v", err))
	}
}
