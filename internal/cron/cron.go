package cron

import (
	"context"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// Scheduler coordinates recurring background reporting, pre-aggregation rollups,
// and automated intelligence analysis.
type Scheduler struct {
	conn           driver.Conn
	rollupInterval time.Duration
	insightsInterval time.Duration
}

// NewScheduler creates a new reporting cron scheduler.
func NewScheduler(conn driver.Conn) *Scheduler {
	return &Scheduler{
		conn:           conn,
		rollupInterval: 10 * time.Minute,
		insightsInterval: 30 * time.Minute,
	}
}

// Start launches the background tickers for rollups and automated insights.
func (s *Scheduler) Start(ctx context.Context) {
	if s.conn == nil {
		log.Println("[Reporting Cron] Warning: ClickHouse connection is nil; skipping scheduler")
		return
	}

	go func() {
		log.Println("[Reporting Cron] Starting background reporting & intelligence schedulers")

		// Run immediate initial execution on startup so UI has fresh data right away
		_ = RunHourlyRollup(ctx, s.conn, 48)
		_ = ComputeDailyInsights(ctx, s.conn)

		rollupTicker := time.NewTicker(s.rollupInterval)
		insightsTicker := time.NewTicker(s.insightsInterval)
		defer rollupTicker.Stop()
		defer insightsTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("[Reporting Cron] Stopping reporting scheduler...")
				return

			case <-rollupTicker.C:
				if err := RunHourlyRollup(ctx, s.conn, 48); err != nil {
					log.Printf("[Reporting Cron] Hourly rollup error: %v", err)
				}

			case <-insightsTicker.C:
				if err := ComputeDailyInsights(ctx, s.conn); err != nil {
					log.Printf("[Reporting Cron] Insights engine error: %v", err)
				}
			}
		}
	}()
}
