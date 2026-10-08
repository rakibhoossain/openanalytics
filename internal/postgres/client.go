package postgres

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPool initializes a thread-safe connection pool to PostgreSQL.
func NewPool(ctx context.Context, connStr string, maxConns int) (*pgxpool.Pool, error) {
	if maxConns <= 0 {
		maxConns = 25
	}

	cfg, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse postgres config: %w", err)
	}

	cfg.MaxConns = int32(maxConns)
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres connection pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	log.Printf("[Postgres] Connected successfully to PostgreSQL (max_conns=%d)", maxConns)
	return pool, nil
}

// Migrate executes initial DDL schema migrations for exchange_rates and shop_integrations.
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS exchange_rates (
			currency_code VARCHAR(10) PRIMARY KEY,
			base_currency VARCHAR(10) NOT NULL DEFAULT 'USD',
			rate NUMERIC(18, 6) NOT NULL,
			source VARCHAR(50) NOT NULL DEFAULT 'openexchangerates',
			raw_timestamp BIGINT NOT NULL DEFAULT 0,
			fetched_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);`,
		`CREATE INDEX IF NOT EXISTS idx_exchange_rates_updated ON exchange_rates(updated_at);`,
		`CREATE TABLE IF NOT EXISTS shop_integrations (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			shop_id UUID NOT NULL,
			tenant_id UUID NOT NULL,
			provider VARCHAR(50) NOT NULL,
			enabled BOOLEAN NOT NULL DEFAULT TRUE,
			credentials JSONB NOT NULL DEFAULT '{}',
			events_whitelist TEXT[] NOT NULL DEFAULT ARRAY[]::TEXT[],
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			CONSTRAINT uq_shop_provider UNIQUE (shop_id, provider)
		);`,
		`CREATE INDEX IF NOT EXISTS idx_shop_integrations_shop ON shop_integrations(shop_id);`,
		`CREATE INDEX IF NOT EXISTS idx_shop_integrations_tenant ON shop_integrations(tenant_id);`,
	}

	for _, q := range queries {
		if _, err := pool.Exec(ctx, q); err != nil {
			return fmt.Errorf("postgres migration query failed [%s]: %w", q, err)
		}
	}

	log.Println("[Postgres] Schemas for exchange_rates and shop_integrations verified/created")
	return nil
}
