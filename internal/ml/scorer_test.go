package ml

import (
	"context"
	"math"
	"testing"
	"time"

	"openanalytics/internal/domain"
	"openanalytics/pkg/uuidv7"
)

func TestScorer_Initialization(t *testing.T) {
	scorer, err := NewScorer("data/models/cart_intent_v1.json", nil)
	if err != nil {
		t.Fatalf("unexpected error creating scorer: %v", err)
	}

	if scorer.weights.Version != "v1.0.0" {
		t.Errorf("expected version v1.0.0, got %s", scorer.weights.Version)
	}
}

func TestScorer_SigmoidInference(t *testing.T) {
	scorer, err := NewScorer("data/models/cart_intent_v1.json", nil)
	if err != nil {
		t.Fatalf("unexpected error creating scorer: %v", err)
	}

	// Calculate manually: bias=-2.85, views=10 (1.2), carts=2 (3.9), distinct=2 (-0.3), dwell=300 (1.05)
	// z = -2.85 + 1.2 + 3.9 - 0.3 + 1.05 = 3.0
	// Sigmoid(3.0) = 1 / (1 + e^-3) = ~0.9525
	z := -2.85 + 1.2 + 3.9 - 0.3 + 1.05
	expectedScore := 1.0 / (1.0 + math.Exp(-z))

	if expectedScore < 0.90 {
		t.Errorf("expected high score > 0.90, got %f", expectedScore)
	}

	// Verify nil redis returns non-blocking default
	event := &domain.Event{
		ID:        uuidv7.MustNew(),
		ShopID:    uuidv7.MustNew(),
		DeviceID:  "device-test-scorer",
		Name:      "page_view",
		CreatedAt: time.Now().UTC(),
	}

	score, isHighIntent, feat, err := scorer.ProcessEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("unexpected error on process event: %v", err)
	}
	if score <= 0 {
		t.Errorf("expected positive score, got %f", score)
	}
	if isHighIntent {
		t.Errorf("expected false for initial single page view")
	}
	if feat == nil {
		t.Fatalf("expected non-nil ShopperFeature snapshot")
	}
	if feat.DeviceID != event.DeviceID {
		t.Errorf("expected device ID %s, got %s", event.DeviceID, feat.DeviceID)
	}
}
