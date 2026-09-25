package session

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"openanalytics/internal/clickhouse"
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
		local wallclock_key = KEYS[2]
		local shops_key = KEYS[3]
		local event_time_ms = tonumber(ARGV[1])
		local timeout_ms = tonumber(ARGV[2])
		local new_session_id = ARGV[3]
		local referrer = ARGV[4]
		local referrer_name = ARGV[5]
		local referrer_type = ARGV[6]
		local entry_path = ARGV[7]
		local tenant_id = ARGV[8]
		local shop_id = ARGV[9]
		local device_id = ARGV[10]
		local os = ARGV[11]
		local browser = ARGV[12]
		local device = ARGV[13]
		local country = ARGV[14]
		local city = ARGV[15]

		local current_id = redis.call('HGET', key, 'id')
		local last_seen_ms = tonumber(redis.call('HGET', key, 'last_seen_ms') or '0')

		-- Update wallclock index for the background session reaper
		if wallclock_key and device_id then
			redis.call('ZADD', wallclock_key, event_time_ms, device_id)
		end
		if shops_key and shop_id then
			redis.call('SADD', shops_key, shop_id)
		end

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

			-- Initialize fresh session with full device & context metadata
			redis.call('HMSET', key,
				'id', new_session_id,
				'started_ms', event_time_ms,
				'last_seen_ms', event_time_ms,
				'events_count', 1,
				'referrer', referrer,
				'referrer_name', referrer_name,
				'referrer_type', referrer_type,
				'entry_path', entry_path,
				'exit_path', entry_path,
				'tenant_id', tenant_id,
				'shop_id', shop_id,
				'device_id', device_id,
				'os', os,
				'browser', browser,
				'device', device,
				'country', country,
				'city', city,
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
			redis.call('HSET', key, 'last_seen_ms', event_time_ms, 'exit_path', entry_path)
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
	wallclockKey := fmt.Sprintf("session:wallclock:%s", event.ShopID.String())
	shopsKey := "session:shops"
	newSessionID := event.SessionID.String()
	if event.SessionID == uuid.Nil {
		newSessionID = uuidv7.MustNew().String()
	}
	eventTimeMs := event.CreatedAt.UnixMilli()
	timeoutMs := m.sessionTimeout.Milliseconds()

	res, err := m.luaScript.Run(ctx, m.rdb, []string{sessionKey, wallclockKey, shopsKey},
		eventTimeMs,
		timeoutMs,
		newSessionID,
		event.Referrer,
		event.ReferrerName,
		event.ReferrerType,
		event.Path,
		event.TenantID.String(),
		event.ShopID.String(),
		event.DeviceID,
		event.OS,
		event.Browser,
		event.Device,
		event.Country,
		event.City,
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
			_ = m.rdb.HIncrBy(ctx, sessionKey, "total_revenue", *event.Revenue).Err()
		}
	}

	var closedSession *domain.Session
	if kind == "boundary" && len(res) >= 16 {
		closedID, _ := uuid.Parse(fmt.Sprint(res[5]))
		var startedMs, endedMs int64
		_, _ = fmt.Sscan(fmt.Sprint(res[6]), &startedMs)
		_, _ = fmt.Sscan(fmt.Sprint(res[7]), &endedMs)
		var eventsCount uint32
		_, _ = fmt.Sscan(fmt.Sprint(res[8]), &eventsCount)

		durationSec := uint32(0)
		if endedMs > startedMs {
			durationSec = uint32((endedMs - startedMs) / 1000)
		}

		var hasCart, hasPurchase uint8
		_, _ = fmt.Sscan(fmt.Sprint(res[13]), &hasCart)
		_, _ = fmt.Sscan(fmt.Sprint(res[14]), &hasPurchase)
		var totRev int64
		_, _ = fmt.Sscan(fmt.Sprint(res[15]), &totRev)

		closedSession = &domain.Session{
			ID:           closedID,
			TenantID:     event.TenantID,
			ShopID:       event.ShopID,
			DeviceID:     event.DeviceID,
			CustomerID:   event.CustomerID,
			StartedAt:    time.UnixMilli(startedMs).UTC(),
			EndedAt:      time.UnixMilli(endedMs).UTC(),
			Duration:     durationSec,
			EntryPath:    fmt.Sprint(res[12]),
			ExitPath:     fmt.Sprint(res[12]),
			Referrer:     fmt.Sprint(res[9]),
			ReferrerName: fmt.Sprint(res[10]),
			ReferrerType: fmt.Sprint(res[11]),
			EventsCount:  eventsCount,
			HasCartAdd:   hasCart > 0,
			HasPurchase:  hasPurchase > 0,
			TotalRevenue: totRev,
		}
	}

	return &IngestResult{
		Kind:          kind,
		SessionID:     sid,
		ClosedSession: closedSession,
	}, nil
}

// ProcessEventLifecycle processes session boundaries, generates synthetic session_start
// and session_end events, and buffers all events and closed sessions to ClickHouse.
// Matches OpenPanel's `events.incoming-event.ts`.
func (m *Manager) ProcessEventLifecycle(ctx context.Context, event *domain.Event, chWriter *clickhouse.BatchWriter) error {
	res, err := m.Ingest(ctx, event)
	if err != nil {
		return err
	}

	if chWriter != nil && res != nil {
		// 1. If previous session timed out or crossed a day boundary, emit synthetic session_end
		if res.Kind == "boundary" && res.ClosedSession != nil {
			endProps := map[string]string{
				"__bounce":   fmt.Sprintf("%t", res.ClosedSession.EventsCount <= 1),
				"__duration": fmt.Sprintf("%d", res.ClosedSession.Duration),
			}
			sessionEndEvent := &domain.Event{
				ID:           uuidv7.MustNew(),
				TenantID:     res.ClosedSession.TenantID,
				ShopID:       res.ClosedSession.ShopID,
				Name:         "session_end",
				DeviceID:     res.ClosedSession.DeviceID,
				SessionID:    res.ClosedSession.ID,
				CustomerID:   res.ClosedSession.CustomerID,
				Path:         res.ClosedSession.ExitPath,
				Origin:       event.Origin,
				Referrer:     res.ClosedSession.Referrer,
				ReferrerName: res.ClosedSession.ReferrerName,
				ReferrerType: res.ClosedSession.ReferrerType,
				OS:           event.OS,
				Browser:      event.Browser,
				Device:       event.Device,
				Country:      event.Country,
				City:         event.City,
				Latitude:     event.Latitude,
				Longitude:    event.Longitude,
				Properties:   endProps,
				CreatedAt:    res.ClosedSession.EndedAt,
			}
			_ = chWriter.AddEvent(ctx, sessionEndEvent)
			chWriter.AddSession(res.ClosedSession)
		}

		// 2. If a new session began (new or boundary), emit synthetic session_start event (-100ms)
		if res.Kind == "new" || res.Kind == "boundary" {
			sessionStartEvent := &domain.Event{
				ID:           uuidv7.MustNew(),
				TenantID:     event.TenantID,
				ShopID:       event.ShopID,
				Name:         "session_start",
				DeviceID:     event.DeviceID,
				SessionID:    res.SessionID,
				CustomerID:   event.CustomerID,
				Path:         event.Path,
				Origin:       event.Origin,
				Referrer:     event.Referrer,
				ReferrerName: event.ReferrerName,
				ReferrerType: event.ReferrerType,
				OS:           event.OS,
				Browser:      event.Browser,
				Device:       event.Device,
				Country:      event.Country,
				City:         event.City,
				Latitude:     event.Latitude,
				Longitude:    event.Longitude,
				Properties:   map[string]string{},
				CreatedAt:    event.CreatedAt.Add(-100 * time.Millisecond),
			}
			_ = chWriter.AddEvent(ctx, sessionStartEvent)
		}

		// 3. Buffer original event
		return chWriter.AddEvent(ctx, event)
	}

	return nil
}

// GetActiveSessionsCount returns the count of shoppers active within the session timeout window.
func (m *Manager) GetActiveSessionsCount(ctx context.Context, shopID uuid.UUID) (int64, error) {
	if m.rdb == nil {
		return 0, nil
	}
	wallclockKey := fmt.Sprintf("session:wallclock:%s", shopID.String())
	minScore := fmt.Sprintf("%d", time.Now().Add(-m.sessionTimeout).UnixMilli())
	maxScore := "+inf"
	return m.rdb.ZCount(ctx, wallclockKey, minScore, maxScore).Result()
}
