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

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"openanalytics/internal/domain"
)

// GenericModelWeights defines the coefficients for behavioral logistic models.
type GenericModelWeights struct {
	Version      string             `json:"version"`
	ModelType    string             `json:"model_type"`
	Features     []string           `json:"features"`
	Coefficients map[string]float64 `json:"coefficients"`
	Thresholds   map[string]float64 `json:"thresholds"`
}

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

// Scorer evaluates real-time shopper purchase propensity, churn risk, and price sensitivity.
type Scorer struct {
	weights      IntentModelWeights
	churnWeights GenericModelWeights
	priceWeights GenericModelWeights
	rdb          *redis.Client
	mu           sync.RWMutex
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
	// Default calibrated weights for Intent
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

	// Default Churn weights
	churnWeights := GenericModelWeights{
		Version:   "v1.1.0",
		ModelType: "calibrated_churn_risk",
		Coefficients: map[string]float64{
			"bias":               1.15,
			"views_count":        -0.08,
			"cart_adds_count":    -2.10,
			"distinct_products":  0.05,
			"dwell_time_seconds": -0.005,
			"avg_scroll_depth":   -1.85,
		},
		Thresholds: map[string]float64{"low_risk": 0.3, "medium_risk": 0.6, "high_risk": 0.8},
	}

	// Default Price Sensitivity weights
	priceWeights := GenericModelWeights{
		Version:   "v1.1.0",
		ModelType: "calibrated_price_sensitivity",
		Coefficients: map[string]float64{
			"bias":               -1.65,
			"views_count":        0.05,
			"sale_view_ratio":    3.85,
			"distinct_products":  0.18,
			"dwell_time_seconds": 0.002,
			"cart_adds_count":    0.35,
		},
		Thresholds: map[string]float64{"low": 0.35, "moderate": 0.65, "price_hunter": 0.85},
	}

	// Try loading from JSON configuration if exists
	jsonPath := modelPath
	if len(jsonPath) > 5 && jsonPath[len(jsonPath)-5:] == ".onnx" {
		jsonPath = jsonPath[:len(jsonPath)-5] + ".json"
	}

	if data, err := os.ReadFile(jsonPath); err == nil {
		if err := json.Unmarshal(data, &weights); err != nil {
			log.Printf("[ML Scorer] Warning: failed to parse weights from %s: %v, using defaults", jsonPath, err)
		} else {
			log.Printf("[ML Scorer] Loaded model weights version %s from %s", weights.Version, jsonPath)
		}
	} else {
		log.Printf("[ML Scorer] Model file %s not found; using calibrated default weights", jsonPath)
	}

	// Try loading churn predictor
	dir := ""
	for i := len(jsonPath) - 1; i >= 0; i-- {
		if jsonPath[i] == '/' || jsonPath[i] == '\\' {
			dir = jsonPath[:i]
			break
		}
	}
	if dir != "" {
		churnPath := dir + "/churn_predictor_v1.json"
		if cData, err := os.ReadFile(churnPath); err == nil {
			_ = json.Unmarshal(cData, &churnWeights)
		}
		pricePath := dir + "/price_sensitivity_v1.json"
		if pData, err := os.ReadFile(pricePath); err == nil {
			_ = json.Unmarshal(pData, &priceWeights)
		}
	}

	return &Scorer{
		weights:      weights,
		churnWeights: churnWeights,
		priceWeights: priceWeights,
		rdb:          rdb,
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

	if event.SessionID != uuid.Nil {
		pipe.HSet(ctx, featureKey, "session_id", event.SessionID.String())
	}
	if event.CustomerID != nil && *event.CustomerID != uuid.Nil {
		pipe.HSet(ctx, featureKey, "customer_id", event.CustomerID.String())
	}
	if event.CartID != nil && *event.CartID != uuid.Nil {
		pipe.HSet(ctx, featureKey, "cart_id", event.CartID.String())
	}
	if event.Revenue != nil {
		pipe.HIncrBy(ctx, featureKey, "cart_value_cents", *event.Revenue)
	}
	if event.Country != "" {
		pipe.HSet(ctx, featureKey, "country", event.Country)
	}
	if event.City != "" {
		pipe.HSet(ctx, featureKey, "city", event.City)
	}
	if event.Path != "" {
		pipe.HSet(ctx, featureKey, "path", event.Path)
	}
	if event.Device != "" {
		pipe.HSet(ctx, featureKey, "device", event.Device)
	}
	if event.Browser != "" {
		pipe.HSet(ctx, featureKey, "browser", event.Browser)
	}
	if event.OS != "" {
		pipe.HSet(ctx, featureKey, "os", event.OS)
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

	// 3. Compute multi-model inference scores: Sigmoid(z)
	s.mu.RLock()
	coefs := s.weights.Coefficients
	zIntent := coefs["bias"] +
		coefs["views_count"]*float64(views) +
		coefs["cart_adds_count"]*float64(carts) +
		coefs["distinct_products"]*float64(distinctProducts) +
		coefs["dwell_time_seconds"]*float64(dwellSeconds)
	highThreshold := s.weights.Thresholds.High

	cCoefs := s.churnWeights.Coefficients
	zChurn := cCoefs["bias"] +
		cCoefs["views_count"]*float64(views) +
		cCoefs["cart_adds_count"]*float64(carts) +
		cCoefs["distinct_products"]*float64(distinctProducts) +
		cCoefs["dwell_time_seconds"]*float64(dwellSeconds) +
		cCoefs["avg_scroll_depth"]*0.70

	pCoefs := s.priceWeights.Coefficients
	zPrice := pCoefs["bias"] +
		pCoefs["views_count"]*float64(views) +
		pCoefs["sale_view_ratio"]*0.25 +
		pCoefs["distinct_products"]*float64(distinctProducts) +
		pCoefs["dwell_time_seconds"]*float64(dwellSeconds) +
		pCoefs["cart_adds_count"]*float64(carts)
	s.mu.RUnlock()

	score := 1.0 / (1.0 + math.Exp(-zIntent))
	churnScore := 1.0 / (1.0 + math.Exp(-zChurn))
	priceScore := 1.0 / (1.0 + math.Exp(-zPrice))
	isHighIntent := score >= highThreshold

	// 4. Cache scores in Redis for live marketing or checkout triggers
	intentKey := fmt.Sprintf("shopper:intent:%s:%s", event.ShopID.String(), event.DeviceID)
	churnKey := fmt.Sprintf("shopper:churn:%s:%s", event.ShopID.String(), event.DeviceID)
	priceKey := fmt.Sprintf("shopper:price:%s:%s", event.ShopID.String(), event.DeviceID)

	scorePipe := s.rdb.Pipeline()
	scorePipe.Set(ctx, intentKey, fmt.Sprintf("%.4f", score), 30*time.Minute)
	scorePipe.Set(ctx, churnKey, fmt.Sprintf("%.4f", churnScore), 30*time.Minute)
	scorePipe.Set(ctx, priceKey, fmt.Sprintf("%.4f", priceScore), 30*time.Minute)

	if isHighIntent {
		triggerKey := fmt.Sprintf("shopper:high_intent:%s:%s", event.ShopID.String(), event.DeviceID)
		scorePipe.Set(ctx, triggerKey, "1", 15*time.Minute)
	}
	if churnScore >= 0.60 && carts > 0 {
		abandonKey := fmt.Sprintf("shopper:abandon_risk:%s:%s", event.ShopID.String(), event.DeviceID)
		scorePipe.Set(ctx, abandonKey, "1", 15*time.Minute)
	}
	_, _ = scorePipe.Exec(ctx)

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
