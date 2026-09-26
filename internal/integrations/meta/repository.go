package meta

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Repository manages persistence and caching of integration credentials.
type Repository struct {
	conn driver.Conn
	rdb  *redis.Client
}

// NewRepository initializes a Repository with ClickHouse and Redis dependencies.
func NewRepository(conn driver.Conn, rdb *redis.Client) *Repository {
	return &Repository{
		conn: conn,
		rdb:  rdb,
	}
}

// GetMetaIntegration retrieves the Meta CAPI integration for a shop.
// Checks Redis cache first (sub-millisecond), falling back to ClickHouse.
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

	if r.conn == nil {
		return nil, nil
	}

	// 2. Query ClickHouse
	query := `
		SELECT shop_id, tenant_id, provider, enabled, credentials, events_whitelist, updated_at
		FROM openpanel.shop_integrations
		WHERE shop_id = ? AND provider = 'meta_capi'
		ORDER BY updated_at DESC
		LIMIT 1
	`
	row := r.conn.QueryRow(ctx, query, shopID)

	var item ShopIntegration
	var enabledVal uint8
	var credStr string
	if err := row.Scan(&item.ShopID, &item.TenantID, &item.Provider, &enabledVal, &credStr, &item.EventsWhitelist, &item.UpdatedAt); err != nil {
		// Cache negative lookup in Redis for 1 minute to prevent query flooding
		if r.rdb != nil {
			r.rdb.Set(ctx, redisKey, "{}", 1*time.Minute)
		}
		return nil, nil
	}

	item.Enabled = (enabledVal == 1)
	if credStr != "" {
		_ = json.Unmarshal([]byte(credStr), &item.Credentials)
	}

	// 3. Populate Redis Cache
	if r.rdb != nil {
		if cacheBytes, err := json.Marshal(item); err == nil {
			r.rdb.Set(ctx, redisKey, cacheBytes, 10*time.Minute)
		}
	}

	return &item, nil
}

// SaveMetaIntegration persists Meta credentials to ClickHouse and updates Redis.
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

	enabledVal := uint8(0)
	if item.Enabled {
		enabledVal = 1
	}

	if r.conn != nil {
		query := `
			INSERT INTO openpanel.shop_integrations 
			(shop_id, tenant_id, provider, enabled, credentials, events_whitelist, updated_at) 
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`
		if err := r.conn.Exec(ctx, query, item.ShopID, item.TenantID, item.Provider, enabledVal, string(credBytes), item.EventsWhitelist, item.UpdatedAt); err != nil {
			return fmt.Errorf("failed to write integration to ClickHouse: %w", err)
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
