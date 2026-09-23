package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"openanalytics/internal/domain"
	"openanalytics/pkg/uuidv7"
)

const (
	// DefaultSessionTimeout is the standard rolling inactivity window.
	// CRITICAL(session-timeout): 30 minutes of inactivity creates a session boundary.
	DefaultSessionTimeout = 30 * time.Minute
)

// IngestResult represents the session resolution verdict.
type IngestResult struct {
	Kind          string          // "new", "extend", "boundary"
	SessionID     uuid.UUID
	ClosedSession *domain.Session // Populated if Kind == "boundary"
}

// Manager coordinates distributed session state in Redis across worker replicas.
type Manager struct {
	rdb            *redis.Client
	sessionTimeout time.Duration
	luaScript      *redis.Script
}

// NewManager creates a new distributed session manager.
func NewManager(rdb *redis.Client, timeout time.Duration) *Manager {
	if timeout <= 0 {
		timeout = DefaultSessionTimeout
	}

	// Lua script guarantees atomic session state evaluation and referrer inheritance
	// across horizontally scaled worker instances.
	// CRITICAL(atomic-state): Avoids race conditions when multiple worker replicas receive rapid events from the same shopper.
	script := redis.NewScript(`
		local key = KEYS[1]
		local event_time_ms = tonumber(ARGV[1])
		local timeout_ms = tonumber(ARGV[2])
		local new_session_id = ARGV[3]
		local referrer = ARGV[4]
		local referrer_name = ARGV[5]
		local referrer_type = ARGV[6]
		local entry_path = ARGV[7]

		local current_id = redis.call('HGET', key, 'id')
		local last_seen_ms = tonumber(redis.call('HGET', key, 'last_seen_ms') or '0')

		-- Check if no session exists or if the idle threshold has elapsed
		if not current_id or (event_time_ms - last_seen_ms > timeout_ms) then
			local closed_id = current_id
			local closed_started_ms = redis.call('HGET', key, 'started_ms')
			local closed_last_seen_ms = last_seen_ms
			local closed_events_count = redis.call('HGET', key, 'events_count')
			local closed_referrer = redis.call('HGET', key, 'referrer')
			local closed_referrer_name = redis.call('HGET', key, 'referrer_name')
			local closed_referrer_type = redis.call('HGET', key, 'referrer_type')
			local closed_entry_path = redis.call('HGET', key, 'entry_path')
			local closed_has_cart = redis.call('HGET', key, 'has_cart_add')
			local closed_has_purchase = redis.call('HGET', key, 'has_purchase')
			local closed_revenue = redis.call('HGET', key, 'total_revenue')

			-- Initialize fresh session
			redis.call('HMSET', key,
				'id', new_session_id,
				'started_ms', event_time_ms,
				'last_seen_ms', event_time_ms,
				'events_count', 1,
				'referrer', referrer,
				'referrer_name', referrer_name,
				'referrer_type', referrer_type,
				'entry_path', entry_path,
				'has_cart_add', 0,
				'has_purchase', 0,
				'total_revenue', 0
			)
			redis.call('EXPIRE', key, math.floor(timeout_ms / 1000) * 2)

			if closed_id then
				return { 'boundary', new_session_id, referrer, referrer_name, referrer_type,
					closed_id, closed_started_ms, closed_last_seen_ms, closed_events_count or '1',
					closed_referrer or '', closed_referrer_name or '', closed_referrer_type or '',
					closed_entry_path or '', closed_has_cart or '0', closed_has_purchase or '0', closed_revenue or '0' }
			else
				return { 'new', new_session_id, referrer, referrer_name, referrer_type }
			end
		else
			-- Extend existing session
			redis.call('HSET', key, 'last_seen_ms', event_time_ms)
			redis.call('HINCRBY', key, 'events_count', 1)
			redis.call('EXPIRE', key, math.floor(timeout_ms / 1000) * 2)

			local original_referrer = redis.call('HGET', key, 'referrer') or referrer
			local original_referrer_name = redis.call('HGET', key, 'referrer_name') or referrer_name
			local original_referrer_type = redis.call('HGET', key, 'referrer_type') or referrer_type

			return { 'extend', current_id, original_referrer, original_referrer_name, original_referrer_type }
		end
	`)

	return &Manager{
		rdb:            rdb,
		sessionTimeout: timeout,
		luaScript:      script,
	}
}

// Ingest processes an event, resolves its session lifecycle, and inherits initial session referrer fields.
func (m *Manager) Ingest(ctx context.Context, event *domain.Event) (*IngestResult, error) {
	// WEAK_POINT(fallback-on-nil-redis): If Redis connection fails, fail-open by generating fresh session ID
	// to prevent blocking event stream processing.
	if m.rdb == nil {
		newID := uuidv7.MustNew()
		event.SessionID = newID
		return &IngestResult{
			Kind:      "new",
			SessionID: newID,
		}, nil
	}

	sessionKey := fmt.Sprintf("session:%s:%s", event.ShopID.String(), event.DeviceID)
	newSessionID := uuidv7.MustNew().String()
	eventTimeMs := event.CreatedAt.UnixMilli()
	timeoutMs := m.sessionTimeout.Milliseconds()

	res, err := m.luaScript.Run(ctx, m.rdb, []string{sessionKey},
		eventTimeMs,
		timeoutMs,
		newSessionID,
		event.Referrer,
		event.ReferrerName,
		event.ReferrerType,
		event.Path,
	).Slice()

	if err != nil {
		// CRITICAL(redis-failure-tolerance): Gracefully degrade on Redis network failure.
		fallbackID := uuidv7.MustNew()
		event.SessionID = fallbackID
		return &IngestResult{
			Kind:      "new",
			SessionID: fallbackID,
		}, nil
	}

	kind := res[0].(string)
	sidStr := res[1].(string)
	sid, _ := uuid.Parse(sidStr)
	event.SessionID = sid

	// Inherit initial session referrer attributes across subsequent mid-session events
	if len(res) > 4 {
		if origRef, ok := res[2].(string); ok && origRef != "" {
			event.Referrer = origRef
		}
		if origName, ok := res[3].(string); ok && origName != "" {
			event.ReferrerName = origName
		}
		if origType, ok := res[4].(string); ok && origType != "" {
			event.ReferrerType = origType
		}
	}

	// Update conversion markers in Redis
	if event.Name == "add_to_cart" {
		_ = m.rdb.HSet(ctx, sessionKey, "has_cart_add", 1).Err()
	}
	if event.Name == "purchase" || event.Name == "order_completed" {
		_ = m.rdb.HSet(ctx, sessionKey, "has_purchase", 1).Err()
		if event.Revenue != nil {
			_ = m.rdb.HIncrByFloat(ctx, sessionKey, "total_revenue", *event.Revenue).Err()
		}
	}

	var closedSession *domain.Session
	if kind == "boundary" && len(res) >= 17 {
		closedID, _ := uuid.Parse(res[5].(string))
		var startedMs, endedMs int64
		_, _ = fmt.Sscan(res[6].(string), &startedMs)
		_, _ = fmt.Sscan(res[7].(string), &endedMs)
		var eventsCount uint32
		_, _ = fmt.Sscan(res[8].(string), &eventsCount)

		durationSec := uint32(0)
		if endedMs > startedMs {
			durationSec = uint32((endedMs - startedMs) / 1000)
		}

		closedSession = &domain.Session{
			ID:           closedID,
			TenantID:     event.TenantID,
			ShopID:       event.ShopID,
			DeviceID:     event.DeviceID,
			CustomerID:   event.CustomerID,
			StartedAt:    time.UnixMilli(startedMs).UTC(),
			EndedAt:      time.UnixMilli(endedMs).UTC(),
			Duration:     durationSec,
			EntryPath:    res[12].(string),
			ExitPath:     event.Path,
			Referrer:     res[9].(string),
			ReferrerName: res[10].(string),
			ReferrerType: res[11].(string),
			EventsCount:  eventsCount,
		}
	}

	return &IngestResult{
		Kind:          kind,
		SessionID:     sid,
		ClosedSession: closedSession,
	}, nil
}
