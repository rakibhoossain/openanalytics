package cron

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"

	"openanalytics/pkg/uuidv7"
)

// ComputeDailyInsights runs the automated intelligence modules across active shops
// and stores rendered insight cards into openpanel.project_insights.
func ComputeDailyInsights(ctx context.Context, conn driver.Conn) error {
	if conn == nil {
		return fmt.Errorf("clickhouse connection is nil")
	}

	// 1. Discover all active shops with activity in the last 7 days
	shopQuery := `
		SELECT DISTINCT shop_id, tenant_id
		FROM openpanel.events
		WHERE created_at >= now() - INTERVAL 7 DAY;
	`
	rows, err := conn.Query(ctx, shopQuery)
	if err != nil {
		return fmt.Errorf("failed to query active shops: %w", err)
	}
	defer rows.Close()

	type shopPair struct {
		shopID   uuid.UUID
		tenantID uuid.UUID
	}
	var shops []shopPair
	for rows.Next() {
		var sp shopPair
		if err := rows.Scan(&sp.shopID, &sp.tenantID); err != nil {
			log.Printf("[Insights Engine] Failed to scan shop: %v", err)
			continue
		}
		shops = append(shops, sp)
	}

	totalInsights := 0
	for _, sp := range shops {
		cards, err := evaluateShopInsights(ctx, conn, sp.shopID, sp.tenantID)
		if err != nil {
			log.Printf("[Insights Engine] Error evaluating shop %s: %v", sp.shopID, err)
			continue
		}

		if len(cards) > 0 {
			if err := persistInsightCards(ctx, conn, cards); err != nil {
				log.Printf("[Insights Engine] Error saving insights for shop %s: %v", sp.shopID, err)
			} else {
				totalInsights += len(cards)
			}
		}
	}

	log.Printf("[Insights Engine] Successfully computed %d automated intelligence cards across %d shops", totalInsights, len(shops))
	return nil
}

func evaluateShopInsights(ctx context.Context, conn driver.Conn, shopID, tenantID uuid.UUID) ([]*InsightCard, error) {
	var cards []*InsightCard
	now := time.Now().UTC()

	// ------------------------------------------------------------------
	// Module 1: Referrers Surge / Drop Detection
	// ------------------------------------------------------------------
	refQuery := `
		SELECT
			if(referrer_name = '', 'Direct / None', referrer_name) AS ref_name,
			toFloat64(countIf(created_at >= now() - INTERVAL 24 HOUR)) AS cur_count,
			round(toFloat64(countIf(created_at >= now() - INTERVAL 8 DAY AND created_at < now() - INTERVAL 24 HOUR)) / 7.0, 1) AS base_count
		FROM openpanel.events
		WHERE shop_id = ? AND created_at >= now() - INTERVAL 8 DAY
		GROUP BY ref_name
		HAVING cur_count >= 1 OR base_count >= 1
		ORDER BY cur_count DESC
		LIMIT 10;
	`
	refRows, err := conn.Query(ctx, refQuery, shopID)
	if err != nil {
		log.Printf("[Insights Engine] Referrer query failed for shop %s: %v", shopID, err)
	} else {
		for refRows.Next() {
			var refName string
			var curCount, baseCount float64
			if err := refRows.Scan(&refName, &curCount, &baseCount); err != nil {
				log.Printf("[Insights Engine] Referrer scan failed: %v", err)
				continue
			}
			card := generateReferrerCard(shopID, tenantID, refName, curCount, baseCount, now)
			if card != nil {
				cards = append(cards, card)
			}
		}
		refRows.Close()
	}

	// ------------------------------------------------------------------
	// Module 2: Cart Velocity & Conversion Trends
	// ------------------------------------------------------------------
	cartQuery := `
		SELECT
			toFloat64(countIf(name = 'add_to_cart' AND created_at >= now() - INTERVAL 24 HOUR)) AS cur_carts,
			round(toFloat64(countIf(name = 'add_to_cart' AND created_at >= now() - INTERVAL 8 DAY AND created_at < now() - INTERVAL 24 HOUR)) / 7.0, 1) AS base_carts,
			toFloat64(sumIf(ifNull(revenue, 0), created_at >= now() - INTERVAL 24 HOUR)) / 100.0 AS cur_rev,
			round(toFloat64(sumIf(ifNull(revenue, 0), created_at >= now() - INTERVAL 8 DAY AND created_at < now() - INTERVAL 24 HOUR)) / 700.0, 2) AS base_rev
		FROM openpanel.events
		WHERE shop_id = ? AND created_at >= now() - INTERVAL 8 DAY;
	`
	var curCarts, baseCarts, curRev, baseRev float64
	if err := conn.QueryRow(ctx, cartQuery, shopID).Scan(&curCarts, &baseCarts, &curRev, &baseRev); err != nil {
		log.Printf("[Insights Engine] Cart query/scan failed for shop %s: %v", shopID, err)
	} else {
		if curCarts > 0 || baseCarts > 0 {
			pct := 0.0
			dir := "flat"
			if baseCarts > 0 {
				pct = ((curCarts - baseCarts) / baseCarts) * 100
				if pct > 5 {
					dir = "up"
				} else if pct < -5 {
					dir = "down"
				}
			} else {
				dir = "up"
				pct = 100
			}

			title := fmt.Sprintf("Cart additions %s %.0f%% (24h)", arrow(dir), math.Abs(pct))
			if dir == "flat" {
				title = fmt.Sprintf("Cart additions steady at %.0f", curCarts)
			}

			cards = append(cards, &InsightCard{
				ID:           uuidv7.MustNew(),
				ShopID:       shopID,
				TenantID:     tenantID,
				ModuleKey:    "cart_trends",
				DimensionKey: "cart_adds:24h",
				WindowKind:   "rolling_24h",
				Title:        title,
				Summary:      fmt.Sprintf("%.0f cart additions in the last 24h compared to %.1f daily baseline (Revenue: $%.2f).", curCarts, baseCarts, curRev),
				Direction:    dir,
				ChangePct:    pct,
				CurrentVal:   curCarts,
				CompareVal:   baseCarts,
				ImpactScore:  math.Abs(pct) * math.Log1p(curCarts),
				CreatedAt:    now,
			})
		}
	}

	// ------------------------------------------------------------------
	// Module 3: Device Conversion Breakdown
	// ------------------------------------------------------------------
	devQuery := `
		SELECT
			if(device = '', 'desktop', device) AS dev_type,
			toFloat64(countIf(created_at >= now() - INTERVAL 24 HOUR)) AS cur_events,
			toFloat64(countIf(name = 'purchase' AND created_at >= now() - INTERVAL 24 HOUR)) AS cur_orders
		FROM openpanel.events
		WHERE shop_id = ? AND created_at >= now() - INTERVAL 24 HOUR
		GROUP BY dev_type
		HAVING cur_events >= 5;
	`
	devRows, err := conn.Query(ctx, devQuery, shopID)
	if err != nil {
		log.Printf("[Insights Engine] Device query failed for shop %s: %v", shopID, err)
	} else {
		for devRows.Next() {
			var devType string
			var curEvents, curOrders float64
			if err := devRows.Scan(&devType, &curEvents, &curOrders); err != nil {
				log.Printf("[Insights Engine] Device scan failed: %v", err)
				continue
			}
			cvr := 0.0
			if curEvents > 0 {
				cvr = (curOrders / curEvents) * 100
			}
			cards = append(cards, &InsightCard{
				ID:           uuidv7.MustNew(),
				ShopID:       shopID,
				TenantID:     tenantID,
				ModuleKey:    "devices",
				DimensionKey: fmt.Sprintf("device:%s", devType),
				WindowKind:   "rolling_24h",
				Title:        fmt.Sprintf("%s device activity (%.0f events, %.1f%% orders)", devType, curEvents, cvr),
				Summary:      fmt.Sprintf("Shoppers on %s generated %.0f total interactions and %.0f completed orders in the last 24 hours.", devType, curEvents, curOrders),
				Direction:    "flat",
				ChangePct:    cvr,
				CurrentVal:   curEvents,
				CompareVal:   curOrders,
				ImpactScore:  curEvents,
				CreatedAt:    now,
			})
		}
		devRows.Close()
	}

	// ------------------------------------------------------------------
	// Module 4: Top Pages / Products Popularity
	// ------------------------------------------------------------------
	pageQuery := `
		SELECT
			if(path = '', '/', path) AS page_path,
			toFloat64(countIf(created_at >= now() - INTERVAL 24 HOUR)) AS cur_views,
			round(toFloat64(countIf(created_at >= now() - INTERVAL 8 DAY AND created_at < now() - INTERVAL 24 HOUR)) / 7.0, 1) AS base_views
		FROM openpanel.events
		WHERE shop_id = ? AND created_at >= now() - INTERVAL 8 DAY AND path != ''
		GROUP BY page_path
		HAVING cur_views >= 5
		ORDER BY cur_views DESC
		LIMIT 5;
	`
	pageRows, err := conn.Query(ctx, pageQuery, shopID)
	if err != nil {
		log.Printf("[Insights Engine] Page query failed for shop %s: %v", shopID, err)
	} else {
		for pageRows.Next() {
			var pagePath string
			var curViews, baseViews float64
			if err := pageRows.Scan(&pagePath, &curViews, &baseViews); err != nil {
				log.Printf("[Insights Engine] Page scan failed: %v", err)
				continue
			}
			pct := 0.0
			dir := "flat"
			if baseViews > 0 {
				pct = ((curViews - baseViews) / baseViews) * 100
				if pct > 5 {
					dir = "up"
				} else if pct < -5 {
					dir = "down"
				}
			} else {
				dir = "up"
				pct = 100.0
			}

			cards = append(cards, &InsightCard{
				ID:           uuidv7.MustNew(),
				ShopID:       shopID,
				TenantID:     tenantID,
				ModuleKey:    "top_pages",
				DimensionKey: fmt.Sprintf("page:%s", pagePath),
				WindowKind:   "rolling_24h",
				Title:        fmt.Sprintf("High velocity page: %s", pagePath),
				Summary:      fmt.Sprintf("%.0f visits in the last 24h (Daily baseline: %.1f). Trending %s.", curViews, baseViews, dir),
				Direction:    dir,
				ChangePct:    pct,
				CurrentVal:   curViews,
				CompareVal:   baseViews,
				ImpactScore:  curViews * 1.2,
				CreatedAt:    now,
			})
		}
		pageRows.Close()
	}

	return cards, nil
}

func generateReferrerCard(shopID, tenantID uuid.UUID, refName string, current, baseline float64, now time.Time) *InsightCard {
	if current < 1 && baseline < 1 {
		return nil
	}

	pct := 0.0
	dir := "flat"
	isNew := baseline == 0 && current > 0

	if isNew {
		dir = "up"
		pct = 100.0
	} else if baseline > 0 {
		pct = ((current - baseline) / baseline) * 100.0
		if pct > 5 {
			dir = "up"
		} else if pct < -5 {
			dir = "down"
		}
	}

	var title string
	if isNew {
		title = fmt.Sprintf("New acquisition source: %s", refName)
	} else {
		title = fmt.Sprintf("Traffic from %s %s %.0f%% (24h)", refName, arrow(dir), math.Abs(pct))
	}

	summary := fmt.Sprintf("Last 24h: %.0f sessions vs %.1f daily baseline.", current, baseline)

	return &InsightCard{
		ID:           uuidv7.MustNew(),
		ShopID:       shopID,
		TenantID:     tenantID,
		ModuleKey:    "referrers",
		DimensionKey: fmt.Sprintf("referrer:%s", refName),
		WindowKind:   "rolling_24h",
		Title:        title,
		Summary:      summary,
		Direction:    dir,
		ChangePct:    pct,
		CurrentVal:   current,
		CompareVal:   baseline,
		ImpactScore:  math.Abs(pct) * math.Log1p(current),
		CreatedAt:    now,
	}
}

func persistInsightCards(ctx context.Context, conn driver.Conn, cards []*InsightCard) error {
	query := `
		INSERT INTO openpanel.project_insights (
			id, shop_id, tenant_id, module_key, dimension_key, window_kind,
			title, summary, direction, change_pct, current_val, compare_val,
			impact_score, created_at
		)
	`
	batch, err := conn.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare insights batch: %w", err)
	}

	for _, c := range cards {
		err := batch.Append(
			c.ID,
			c.ShopID,
			c.TenantID,
			c.ModuleKey,
			c.DimensionKey,
			c.WindowKind,
			c.Title,
			c.Summary,
			c.Direction,
			c.ChangePct,
			c.CurrentVal,
			c.CompareVal,
			c.ImpactScore,
			c.CreatedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to append insight card: %w", err)
		}
	}

	return batch.Send()
}

func arrow(dir string) string {
	switch dir {
	case "up":
		return "↑"
	case "down":
		return "↓"
	default:
		return "→"
	}
}
