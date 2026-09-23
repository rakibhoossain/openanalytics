package cron

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
)

// RunHourlyRollup pre-aggregates raw telemetry events into openpanel.hourly_metrics
// for the given lookback window. ReplacingMergeTree ensures idempotency.
func RunHourlyRollup(ctx context.Context, conn driver.Conn, lookbackHours int) error {
	if conn == nil {
		return fmt.Errorf("clickhouse connection is nil")
	}
	if lookbackHours <= 0 {
		lookbackHours = 48
	}

	since := time.Now().UTC().Add(-time.Duration(lookbackHours) * time.Hour)

	query := `
		INSERT INTO openpanel.hourly_metrics
		SELECT
			shop_id,
			tenant_id,
			toStartOfHour(created_at) AS hour,
			uniq(device_id) AS unique_visitors,
			uniq(session_id) AS sessions_count,
			countIf(name = 'view_product' OR name = 'page_view') AS pageviews,
			countIf(name = 'add_to_cart') AS cart_adds,
			countIf(name = 'purchase' OR name = 'order_completed') AS purchases,
			sum(ifNull(revenue, 0)) AS gross_revenue
		FROM openpanel.events
		WHERE created_at >= ?
		GROUP BY shop_id, tenant_id, hour;
	`

	if err := conn.Exec(ctx, query, since); err != nil {
		return fmt.Errorf("hourly rollup query failed: %w", err)
	}

	log.Printf("[Hourly Rollup] Pre-aggregated metrics successfully (lookback: %d hours)", lookbackHours)
	return nil
}
