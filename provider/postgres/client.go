package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	configprovider "github.com/pipewave-dev/go-pkg/provider/config-provider"
	"github.com/samber/do/v2"
)

func NewDI(i do.Injector) (*pgxpool.Pool, error) {
	cfg := do.MustInvoke[configprovider.ConfigStore](i)
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

	if err := waitForReady(pool); err != nil {
		pool.Close()
		panic(fmt.Sprintf("postgres: failed to ping: %v", err))
	}

	if err := HandleStartupMigration(context.Background(), pool, cfg.Env().AutoMigration); err != nil {
		pool.Close()
		panic(err.Error())
	}

	return pool, nil
}

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

	if err := waitForReady(pool); err != nil {
		pool.Close()
		panic(fmt.Sprintf("postgres: failed to ping: %v", err))
	}

	if err := HandleStartupMigration(context.Background(), pool, cfg.Env().AutoMigration); err != nil {
		pool.Close()
		panic(err.Error())
	}

	return pool
}

func waitForReady(pool *pgxpool.Pool) error {
	const (
		timeout  = 30 * time.Second
		interval = 500 * time.Millisecond
	)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var lastErr error

	for {
		lastErr = pool.Ping(ctx)
		if lastErr == nil {
			return nil
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("waited %s for postgres readiness: %w", timeout, lastErr)
			}
			return fmt.Errorf("waited %s for postgres readiness: %w", timeout, ctx.Err())
		case <-ticker.C:
		}
	}
}
