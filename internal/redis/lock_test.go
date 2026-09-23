package redis

import (
	"context"
	"testing"
	"time"
)

func TestLocker_NilRedisFallback(t *testing.T) {
	// WEAK_POINT(nil-redis): Verify fail-open fallback behavior when Redis is unavailable.
	locker := NewLocker(nil, "worker-1")

	ctx := context.Background()
	acquired, err := locker.Acquire(ctx, "cron:reports", 10*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !acquired {
		t.Errorf("expected lock acquisition to succeed on nil redis")
	}

	err = locker.Release(ctx, "cron:reports")
	if err != nil {
		t.Fatalf("unexpected release error: %v", err)
	}
}
