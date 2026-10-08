package currency

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

const (
	RedisRatesKey = "analytics:rates:usd_base"
)

// Service provides high-performance thread-safe currency conversion lookups.
type Service struct {
	pool  *pgxpool.Pool
	rdb   *redis.Client
	mu    sync.RWMutex
	rates map[string]float64
}

// NewService creates a currency service with an in-memory L1 cache, Redis L2, and Postgres L3.
func NewService(pool *pgxpool.Pool, rdb *redis.Client) *Service {
	s := &Service{
		pool: pool,
		rdb:  rdb,
		rates: map[string]float64{
			"USD": 1.0,
			"EUR": 0.89,
			"GBP": 0.77,
			"BDT": 123.0,
			"CAD": 1.42,
			"AUD": 1.44,
		},
	}
	return s
}

// LoadRates populates the in-memory cache from Redis or PostgreSQL.
func (s *Service) LoadRates(ctx context.Context) error {
	// 1. Try Redis Cache
	if s.rdb != nil {
		data, err := s.rdb.Get(ctx, RedisRatesKey).Result()
		if err == nil && data != "" {
			var redisRates map[string]float64
			if err := json.Unmarshal([]byte(data), &redisRates); err == nil && len(redisRates) > 0 {
				s.SetRates(redisRates)
				log.Printf("[Currency] Loaded %d exchange rates from Redis cache", len(redisRates))
				return nil
			}
		}
	}

	// 2. Fallback to PostgreSQL
	if s.pool != nil {
		rows, err := s.pool.Query(ctx, `SELECT currency_code, rate FROM exchange_rates`)
		if err == nil {
			defer rows.Close()
			dbRates := make(map[string]float64)
			for rows.Next() {
				var code string
				var rate float64
				if err := rows.Scan(&code, &rate); err == nil && rate > 0 {
					dbRates[strings.ToUpper(code)] = rate
				}
			}
			if len(dbRates) > 0 {
				s.SetRates(dbRates)
				log.Printf("[Currency] Loaded %d exchange rates from PostgreSQL", len(dbRates))
				// Populate Redis for subsequent hits
				if s.rdb != nil {
					if b, err := json.Marshal(dbRates); err == nil {
						s.rdb.Set(ctx, RedisRatesKey, b, 6*time.Hour)
					}
				}
				return nil
			}
		}
	}

	log.Printf("[Currency] Using default baseline exchange rates (%d currencies)", len(s.rates))
	return nil
}

// SetRates updates the in-memory rate map safely.
func (s *Service) SetRates(newRates map[string]float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, v := range newRates {
		if v > 0 {
			s.rates[strings.ToUpper(k)] = v
		}
	}
	s.rates["USD"] = 1.0
}

// GetRate returns the USD-base exchange rate for a currency code (e.g., BDT -> 123.08).
// Nanosecond in-memory read.
func (s *Service) GetRate(currencyCode string) float64 {
	code := strings.ToUpper(strings.TrimSpace(currencyCode))
	if code == "" || code == "USD" {
		return 1.0
	}

	s.mu.RLock()
	rate, ok := s.rates[code]
	s.mu.RUnlock()

	if ok && rate > 0 {
		return rate
	}
	return 1.0
}

// GetAllRates returns a copy of current rates.
func (s *Service) GetAllRates() map[string]float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	copied := make(map[string]float64, len(s.rates))
	for k, v := range s.rates {
		copied[k] = v
	}
	return copied
}

// ConvertToUSD converts monetary cents from transaction currency to normalized USD base cents.
func (s *Service) ConvertToUSD(cents int64, currencyCode string) int64 {
	code := strings.ToUpper(strings.TrimSpace(currencyCode))
	if code == "" || code == "USD" {
		return cents
	}

	rate := s.GetRate(code)
	if rate <= 0 {
		return cents
	}

	return int64(math.Round(float64(cents) / rate))
}

// ConvertFromUSD converts base USD cents into target shop currency cents.
func (s *Service) ConvertFromUSD(usdCents int64, targetCurrency string) int64 {
	code := strings.ToUpper(strings.TrimSpace(targetCurrency))
	if code == "" || code == "USD" {
		return usdCents
	}

	rate := s.GetRate(code)
	if rate <= 0 {
		return usdCents
	}

	return int64(math.Round(float64(usdCents) * rate))
}
