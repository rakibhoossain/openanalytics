package session

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"openanalytics/internal/clickhouse"
	"openanalytics/internal/domain"
	"openanalytics/pkg/uuidv7"
)

// Reaper periodically scans Redis for idle sessions whose deadman timeout (30 min)
// has elapsed without any subsequent event, closes them, emits synthetic session_end
// events, and flushes completed Session records into ClickHouse.
// Matches OpenPanel's `cron.session-reaper.ts`.
type Reaper struct {
	rdb            *redis.Client
	chWriter       *clickhouse.BatchWriter
	sessionTimeout time.Duration
	tickerInterval time.Duration
}

// NewReaper initializes the background session reaper.
func NewReaper(rdb *redis.Client, chWriter *clickhouse.BatchWriter, sessionTimeout, tickerInterval time.Duration) *Reaper {
	if tickerInterval <= 0 {
		tickerInterval = 1 * time.Minute
	}
	if sessionTimeout <= 0 {
		sessionTimeout = DefaultSessionTimeout
	}

	return &Reaper{
		rdb:            rdb,
		chWriter:       chWriter,
		sessionTimeout: sessionTimeout,
		tickerInterval: tickerInterval,
	}
}

// Start launches the reaper loop in a background goroutine.
func (r *Reaper) Start(ctx context.Context) {
	if r.rdb == nil || r.chWriter == nil {
		return
	}

	go func() {
		log.Printf("[Session Reaper] Background reaper active (interval: %v, deadman: %v)", r.tickerInterval, r.sessionTimeout)
		ticker := time.NewTicker(r.tickerInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("[Session Reaper] Stopping session reaper...")
				return
			case <-ticker.C:
				r.reapIdleSessions(ctx)
			}
		}
	}()
}

// reapIdleSessions finds and closes expired sessions across all active shops.
func (r *Reaper) reapIdleSessions(ctx context.Context) {
	shopIDs, err := r.rdb.SMembers(ctx, "session:shops").Result()
	if err != nil || len(shopIDs) == 0 {
		return
	}

	now := time.Now().UTC()
	cutoffMs := now.Add(-r.sessionTimeout).UnixMilli()

	totalReaped := 0

	for _, shopIDStr := range shopIDs {
		wallclockKey := fmt.Sprintf("session:wallclock:%s", shopIDStr)

		// Acquire per-shop lock for 45s to avoid duplicate reaping across multi-replica workers
		lockKey := fmt.Sprintf("session:reaper:lock:%s", shopIDStr)
		locked, err := r.rdb.SetNX(ctx, lockKey, "1", 45*time.Second).Result()
		if err != nil || !locked {
			continue
		}

		// Query up to 500 expired device sessions in this shop
		candidates, err := r.rdb.ZRangeByScore(ctx, wallclockKey, &redis.ZRangeBy{
			Min:    "0",
			Max:    strconv.FormatInt(cutoffMs, 10),
			Offset: 0,
			Count:  500,
		}).Result()

		if err != nil || len(candidates) == 0 {
			// Check if shop has any remaining sessions
			count, _ := r.rdb.ZCard(ctx, wallclockKey).Result()
			if count == 0 {
				_ = r.rdb.SRem(ctx, "session:shops", shopIDStr).Err()
			}
			_ = r.rdb.Del(ctx, lockKey).Err()
			continue
		}

		shopUUID, _ := uuid.Parse(shopIDStr)

		for _, deviceID := range candidates {
			sessionKey := fmt.Sprintf("session:%s:%s", shopIDStr, deviceID)
			sData, err := r.rdb.HGetAll(ctx, sessionKey).Result()
			if err != nil || len(sData) == 0 {
				// Orphan index entry; remove from wallclock
				_ = r.rdb.ZRem(ctx, wallclockKey, deviceID).Err()
				continue
			}

			// Parse session data
			sessionID, _ := uuid.Parse(sData["id"])
			tenantUUID, _ := uuid.Parse(sData["tenant_id"])
			startedMs, _ := strconv.ParseInt(sData["started_ms"], 10, 64)
			lastSeenMs, _ := strconv.ParseInt(sData["last_seen_ms"], 10, 64)
			eventsCount, _ := strconv.ParseUint(sData["events_count"], 10, 32)
			hasCart, _ := strconv.ParseUint(sData["has_cart_add"], 10, 8)
			hasPurchase, _ := strconv.ParseUint(sData["has_purchase"], 10, 8)
			totalRevenue, _ := strconv.ParseInt(sData["total_revenue"], 10, 64)

			durationSec := uint32(0)
			if lastSeenMs > startedMs {
				durationSec = uint32((lastSeenMs - startedMs) / 1000)
			}

			endedAt := time.UnixMilli(lastSeenMs).UTC()
			startedAt := time.UnixMilli(startedMs).UTC()

			closedSession := &domain.Session{
				ID:           sessionID,
				TenantID:     tenantUUID,
				ShopID:       shopUUID,
				DeviceID:     deviceID,
				StartedAt:    startedAt,
				EndedAt:      endedAt,
				Duration:     durationSec,
				EntryPath:    sData["entry_path"],
				ExitPath:     sData["exit_path"],
				Referrer:     sData["referrer"],
				ReferrerName: sData["referrer_name"],
				ReferrerType: sData["referrer_type"],
				EventsCount:  uint32(eventsCount),
				HasCartAdd:   hasCart > 0,
				HasPurchase:  hasPurchase > 0,
				TotalRevenue: totalRevenue,
			}

			// 1. Emit synthetic session_end event
			sessionEndEvent := &domain.Event{
				ID:           uuidv7.MustNew(),
				TenantID:     tenantUUID,
				ShopID:       shopUUID,
				Name:         "session_end",
				DeviceID:     deviceID,
				SessionID:    sessionID,
				Path:         closedSession.ExitPath,
				Referrer:     closedSession.Referrer,
				ReferrerName: closedSession.ReferrerName,
				ReferrerType: closedSession.ReferrerType,
				OS:           sData["os"],
				Browser:      sData["browser"],
				Device:       sData["device"],
				Country:      sData["country"],
				City:         sData["city"],
				Properties: map[string]string{
					"__bounce":   fmt.Sprintf("%t", closedSession.EventsCount <= 1),
					"__duration": fmt.Sprintf("%d", closedSession.Duration),
				},
				CreatedAt: endedAt,
			}

			_ = r.chWriter.AddEvent(ctx, sessionEndEvent)
			r.chWriter.AddSession(closedSession)

			// Clean up Redis session hash and wallclock set
			_ = r.rdb.Del(ctx, sessionKey).Err()
			_ = r.rdb.ZRem(ctx, wallclockKey, deviceID).Err()
			totalReaped++
		}

		// Release per-shop lock
		_ = r.rdb.Del(ctx, lockKey).Err()
	}

	if totalReaped > 0 {
		log.Printf("[Session Reaper] Successfully closed and flushed %d idle sessions to ClickHouse", totalReaped)
	}
}
