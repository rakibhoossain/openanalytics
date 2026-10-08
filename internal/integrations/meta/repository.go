package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// Repository manages persistence and caching of integration credentials in PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
	rdb  *redis.Client
}

// NewRepository initializes a Repository with PostgreSQL and Redis dependencies.
func NewRepository(pool *pgxpool.Pool, rdb *redis.Client) *Repository {
	return &Repository{
		pool: pool,
		rdb:  rdb,
	}
}

// GetMetaIntegration retrieves the Meta CAPI integration for a shop.
// Checks Redis cache first (sub-millisecond), falling back to PostgreSQL.
func (r *Repository) GetMetaIntegration(ctx context.Context, shopID uuid.UUID) (*ShopIntegration, error) {
	if shopID == uuid.Nil {
		return nil, nil
	}

	redisKey := fmt.Sprintf("shop:integration:%s:meta_capi", shopID.String())

	// 1. Check Redis Cache
	if r.rdb != nil {
		val, err := r.rdb.Get(ctx, redisKey).Result()
		if err == nil && val != "" {
			if val == "{}" || val == "null" {
				return nil, nil
			}
			var cached ShopIntegration
			if err := json.Unmarshal([]byte(val), &cached); err == nil {
				return &cached, nil
			}
		}
	}

	if r.pool == nil {
		return nil, nil
	}

	// 2. Query PostgreSQL
	query := `
		SELECT shop_id, tenant_id, provider, enabled, credentials, events_whitelist, updated_at
		FROM shop_integrations
		WHERE shop_id = $1 AND provider = 'meta_capi'
		LIMIT 1
	`
	row := r.pool.QueryRow(ctx, query, shopID)

	var item ShopIntegration
	var credBytes []byte
	if err := row.Scan(&item.ShopID, &item.TenantID, &item.Provider, &item.Enabled, &credBytes, &item.EventsWhitelist, &item.UpdatedAt); err != nil {
		// Cache negative lookup in Redis for 1 minute to prevent query flooding
		if r.rdb != nil {
			r.rdb.Set(ctx, redisKey, "{}", 1*time.Minute)
		}
		return nil, nil
	}

	if len(credBytes) > 0 {
		_ = json.Unmarshal(credBytes, &item.Credentials)
	}

	// 3. Populate Redis Cache
	if r.rdb != nil {
		if cacheBytes, err := json.Marshal(item); err == nil {
			r.rdb.Set(ctx, redisKey, cacheBytes, 10*time.Minute)
		}
	}

	return &item, nil
}

// SaveMetaIntegration persists Meta credentials to PostgreSQL with ACID upsert and updates Redis.
func (r *Repository) SaveMetaIntegration(ctx context.Context, item *ShopIntegration) error {
	if item == nil {
		return fmt.Errorf("integration cannot be nil")
	}
	if item.ShopID == uuid.Nil {
		return fmt.Errorf("valid shop_id is required")
	}

	item.Provider = ProviderMetaCAPI
	item.UpdatedAt = time.Now().UTC()

	credBytes, err := json.Marshal(item.Credentials)
	if err != nil {
		return fmt.Errorf("failed to serialize credentials: %w", err)
	}

	if r.pool != nil {
		query := `
			INSERT INTO shop_integrations 
				(shop_id, tenant_id, provider, enabled, credentials, events_whitelist, updated_at) 
			VALUES ($1, $2, $3, $4, $5, $6, NOW())
			ON CONFLICT (shop_id, provider) DO UPDATE SET
				enabled = EXCLUDED.enabled,
				credentials = EXCLUDED.credentials,
				events_whitelist = EXCLUDED.events_whitelist,
				updated_at = NOW()
		`
		if _, err := r.pool.Exec(ctx, query, item.ShopID, item.TenantID, item.Provider, item.Enabled, credBytes, item.EventsWhitelist); err != nil {
			return fmt.Errorf("failed to write integration to PostgreSQL: %w", err)
		}
	}

	// Update Redis cache immediately
	if r.rdb != nil {
		redisKey := fmt.Sprintf("shop:integration:%s:meta_capi", item.ShopID.String())
		if cacheBytes, err := json.Marshal(item); err == nil {
			r.rdb.Set(ctx, redisKey, cacheBytes, 10*time.Minute)
		}
	}

	return nil
}

// InvalidateCache clears the Redis cache for a shop's integration.
func (r *Repository) InvalidateCache(ctx context.Context, shopID uuid.UUID) {
	if r.rdb != nil && shopID != uuid.Nil {
		redisKey := fmt.Sprintf("shop:integration:%s:meta_capi", shopID.String())
		r.rdb.Del(ctx, redisKey)
	}
}
