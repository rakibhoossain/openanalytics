package currency

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

type openExchangeRatesResponse struct {
	Disclaimer string             `json:"disclaimer"`
	License    string             `json:"license"`
	Timestamp  int64              `json:"timestamp"`
	Base       string             `json:"base"`
	Rates      map[string]float64 `json:"rates"`
}

// Scheduler handles periodic fetching and synchronization of currency exchange rates.
type Scheduler struct {
	appID      string
	pool       *pgxpool.Pool
	rdb        *redis.Client
	service    *Service
	interval   time.Duration
	httpClient *http.Client
}

// NewScheduler creates an exchange rate synchronization scheduler.
func NewScheduler(appID string, pool *pgxpool.Pool, rdb *redis.Client, service *Service, intervalHours int) *Scheduler {
	if intervalHours <= 0 {
		intervalHours = 6
	}
	return &Scheduler{
		appID:      appID,
		pool:       pool,
		rdb:        rdb,
		service:    service,
		interval:   time.Duration(intervalHours) * time.Hour,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// Start launches the scheduler in a background goroutine.
func (s *Scheduler) Start(ctx context.Context) {
	if s.appID == "" {
		log.Println("[CurrencyScheduler] OpenExchangeRates app_id not set; automatic sync disabled (using stored rates from PostgreSQL/Redis)")
		return
	}
	go func() {
		log.Printf("[CurrencyScheduler] Starting currency sync scheduler (interval: %v)", s.interval)
		// Run initial synchronization immediately on boot
		if err := s.SyncRates(ctx); err != nil {
			log.Printf("[CurrencyScheduler] Warning: initial sync failed: %v", err)
		}

		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				log.Println("[CurrencyScheduler] Stopping currency scheduler")
				return
			case <-ticker.C:
				if err := s.SyncRates(ctx); err != nil {
					log.Printf("[CurrencyScheduler] Error syncing rates: %v", err)
				}
			}
		}
	}()
}

// SyncRates fetches latest rates from OpenExchangeRates and updates PostgreSQL, Redis, and Memory.
func (s *Scheduler) SyncRates(ctx context.Context) error {
	if s.appID == "" {
		return fmt.Errorf("openexchangerates app_id is empty")
	}

	// 0. Atomic Distributed Mutex across horizontally scaled worker replicas
	if s.rdb != nil {
		lockDuration := s.interval - 5*time.Minute
		if lockDuration < 10*time.Minute {
			lockDuration = 10 * time.Minute
		}
		acquired, err := s.rdb.SetNX(ctx, "lock:cron:currency_sync", "1", lockDuration).Result()
		if err != nil || !acquired {
			log.Println("[CurrencyScheduler] Another worker replica is syncing or recently synced; loading rates from cache")
			if s.service != nil {
				return s.service.LoadRates(ctx)
			}
			return nil
		}
	}

	url := fmt.Sprintf("https://openexchangerates.org/api/latest.json?app_id=%s", s.appID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create http request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http request to openexchangerates failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("openexchangerates returned status %d: %s", resp.StatusCode, string(body))
	}

	var data openExchangeRatesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("failed to decode json response: %w", err)
	}

	if len(data.Rates) == 0 {
		return fmt.Errorf("empty rates received from openexchangerates")
	}

	// 1. Batch Upsert to PostgreSQL
	if s.pool != nil {
		batch := &pgx.Batch{}
		for code, rate := range data.Rates {
			codeClean := strings.ToUpper(strings.TrimSpace(code))
			batch.Queue(`
				INSERT INTO exchange_rates (currency_code, base_currency, rate, source, raw_timestamp, updated_at)
				VALUES ($1, 'USD', $2, 'openexchangerates', $3, NOW())
				ON CONFLICT (currency_code) DO UPDATE SET
					rate = EXCLUDED.rate,
					raw_timestamp = EXCLUDED.raw_timestamp,
					updated_at = NOW()
			`, codeClean, rate, data.Timestamp)
		}

		br := s.pool.SendBatch(ctx, batch)
		if err := br.Close(); err != nil {
			log.Printf("[CurrencyScheduler] Warning: postgres batch upsert error: %v", err)
		} else {
			log.Printf("[CurrencyScheduler] Successfully upserted %d currency rates into PostgreSQL", len(data.Rates))
		}
	}

	// 2. Cache into Redis (TTL 6 Hours)
	if s.rdb != nil {
		if b, err := json.Marshal(data.Rates); err == nil {
			s.rdb.Set(ctx, RedisRatesKey, b, s.interval+1*time.Hour)
		}
	}

	// 3. Update In-Memory Service Cache
	if s.service != nil {
		s.service.SetRates(data.Rates)
	}

	log.Printf("[CurrencyScheduler] Currency exchange rates refreshed successfully (%d currencies, base: %s)", len(data.Rates), data.Base)
	return nil
}
