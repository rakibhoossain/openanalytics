package ml

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"openanalytics/internal/domain"
)

// IntentModelWeights defines the coefficients for the logistic regression / tree intent model.
type IntentModelWeights struct {
	Version      string             `json:"version"`
	ModelType    string             `json:"model_type"`
	Features     []string           `json:"features"`
	Coefficients map[string]float64 `json:"coefficients"`
	Thresholds   struct {
		Low    float64 `json:"low_intent"`
		Medium float64 `json:"medium_intent"`
		High   float64 `json:"high_intent"`
	} `json:"thresholds"`
}

// Scorer evaluates real-time shopper purchase propensity from clickstream events.
type Scorer struct {
	weights IntentModelWeights
	rdb     *redis.Client
	mu      sync.RWMutex
}

// ShopperFeatures holds extracted session features.
type ShopperFeatures struct {
	ViewsCount       int64
	CartAddsCount    int64
	DistinctProducts int64
	DwellSeconds     int64
}

// NewScorer loads model weights and initializes the inference engine.
func NewScorer(modelPath string, rdb *redis.Client) (*Scorer, error) {
	// Default calibrated weights
	weights := IntentModelWeights{
		Version:   "v1.0.0",
		ModelType: "calibrated_logistic_intent",
		Coefficients: map[string]float64{
			"bias":               -2.85,
			"views_count":        0.12,
			"cart_adds_count":    1.95,
			"distinct_products":  -0.15,
			"dwell_time_seconds": 0.0035,
		},
	}
	weights.Thresholds.Low = 0.30
	weights.Thresholds.Medium = 0.65
	weights.Thresholds.High = 0.85

	// Try loading from JSON configuration if exists
	jsonPath := modelPath
	if len(jsonPath) > 5 && jsonPath[len(jsonPath)-5:] == ".onnx" {
		jsonPath = jsonPath[:len(jsonPath)-5] + ".json"
	}

	data, err := os.ReadFile(jsonPath)
	if err == nil {
		if err := json.Unmarshal(data, &weights); err != nil {
			log.Printf("[ML Scorer] Warning: failed to parse weights from %s: %v, using defaults", jsonPath, err)
		} else {
			log.Printf("[ML Scorer] Loaded model weights version %s from %s", weights.Version, jsonPath)
		}
	} else {
		// WEAK_POINT(fallback-model): Use built-in calibrated model coefficients when model file is not on disk.
		log.Printf("[ML Scorer] Model file %s not found; using calibrated default weights", jsonPath)
	}

	return &Scorer{
		weights: weights,
		rdb:     rdb,
	}, nil
}

// ProcessEvent updates the shopper's real-time feature window in Redis, calculates purchase intent,
// and returns the full feature snapshot ready for ClickHouse feature store ingestion.
func (s *Scorer) ProcessEvent(ctx context.Context, event *domain.Event) (float64, bool, *domain.ShopperFeature, error) {
	if s.rdb == nil {
		// Fallback fast scoring without Redis state
		feat := &domain.ShopperFeature{
			TenantID:        event.TenantID,
			ShopID:          event.ShopID,
			DeviceID:        event.DeviceID,
			SessionID:       event.SessionID,
			ViewsCount:      1,
			CartIntentScore: 0.1,
			LastEventAt:     event.CreatedAt,
		}
		return 0.1, false, feat, nil
	}

	featureKey := fmt.Sprintf("shopper:feat:%s:%s", event.ShopID.String(), event.DeviceID)
	nowMs := event.CreatedAt.UnixMilli()

	// 1. Update rolling features in Redis
	pipe := s.rdb.Pipeline()

	if event.Name == "page_view" || event.Name == "view_product" {
		pipe.HIncrBy(ctx, featureKey, "views", 1)
		if event.ProductID != nil {
			// Track distinct products via Redis Set
			prodSetKey := fmt.Sprintf("shopper:prods:%s:%s", event.ShopID.String(), event.DeviceID)
			pipe.SAdd(ctx, prodSetKey, event.ProductID.String())
			pipe.Expire(ctx, prodSetKey, 45*time.Minute)
		}
	} else if event.Name == "add_to_cart" {
		pipe.HIncrBy(ctx, featureKey, "carts", 1)
	}

	pipe.HSetNX(ctx, featureKey, "first_seen_ms", nowMs)
	pipe.HSet(ctx, featureKey, "last_seen_ms", nowMs)
	pipe.Expire(ctx, featureKey, 45*time.Minute)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0.0, false, nil, fmt.Errorf("failed to update shopper features in redis: %w", err)
	}

	// 2. Fetch current aggregated features
	vals, err := s.rdb.HMGet(ctx, featureKey, "views", "carts", "first_seen_ms", "last_seen_ms").Result()
	if err != nil {
		return 0.0, false, nil, err
	}

	var views, carts, firstSeen, lastSeen int64
	if vals[0] != nil {
		fmt.Sscan(vals[0].(string), &views)
	}
	if vals[1] != nil {
		fmt.Sscan(vals[1].(string), &carts)
	}
	if vals[2] != nil {
		fmt.Sscan(vals[2].(string), &firstSeen)
	}
	if vals[3] != nil {
		fmt.Sscan(vals[3].(string), &lastSeen)
	}

	dwellSeconds := int64(0)
	if lastSeen > firstSeen {
		dwellSeconds = (lastSeen - firstSeen) / 1000
	}

	distinctProducts := int64(1)
	prodSetKey := fmt.Sprintf("shopper:prods:%s:%s", event.ShopID.String(), event.DeviceID)
	if scnt, err := s.rdb.SCard(ctx, prodSetKey).Result(); err == nil && scnt > 0 {
		distinctProducts = scnt
	}

	// 3. Compute inference score: Sigmoid(z)
	// CRITICAL(inference-latency): Linear/logistic model computation runs in < 50 nanoseconds in Go.
	s.mu.RLock()
	coefs := s.weights.Coefficients
	z := coefs["bias"] +
		coefs["views_count"]*float64(views) +
		coefs["cart_adds_count"]*float64(carts) +
		coefs["distinct_products"]*float64(distinctProducts) +
		coefs["dwell_time_seconds"]*float64(dwellSeconds)
	highThreshold := s.weights.Thresholds.High
	s.mu.RUnlock()

	score := 1.0 / (1.0 + math.Exp(-z))
	isHighIntent := score >= highThreshold

	// 4. Cache intent score in Redis for live marketing or checkout triggers
	intentKey := fmt.Sprintf("shopper:intent:%s:%s", event.ShopID.String(), event.DeviceID)
	_ = s.rdb.Set(ctx, intentKey, fmt.Sprintf("%.4f", score), 30*time.Minute).Err()

	if isHighIntent {
		// CRITICAL(intent-trigger): Mark high-intent flag for active session
		triggerKey := fmt.Sprintf("shopper:high_intent:%s:%s", event.ShopID.String(), event.DeviceID)
		_ = s.rdb.Set(ctx, triggerKey, "1", 15*time.Minute).Err()
	}

	hasPurchase := uint8(0)
	if event.Name == "purchase" {
		hasPurchase = 1
	}

	feat := &domain.ShopperFeature{
		TenantID:          event.TenantID,
		ShopID:            event.ShopID,
		DeviceID:          event.DeviceID,
		SessionID:         event.SessionID,
		ViewsCount:        uint32(views),
		CartAddsCount:     uint32(carts),
		DistinctProducts:  uint32(distinctProducts),
		TotalDwellSeconds: uint32(dwellSeconds),
		HasPurchase:       hasPurchase,
		CartIntentScore:   float32(score),
		LastEventAt:       event.CreatedAt,
	}

	return score, isHighIntent, feat, nil
}
