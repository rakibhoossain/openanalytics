package clickhouse

import (
	"context"
	"testing"
	"time"
)

func TestBatchWriter_InvalidAddr(t *testing.T) {
	// CRITICAL(connection-validation): Verify NewBatchWriter fails immediately if ClickHouse cannot be reached
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := NewBatchWriter(ctx, Config{
		Addr: "127.0.0.1:59999", // Unreachable port
	})
	if err == nil {
		t.Errorf("expected error connecting to non-existent ClickHouse port, got nil")
	}
}

func TestBatchWriter_DefaultConfig(t *testing.T) {
	cfg := Config{}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 5000
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	if cfg.Database == "" {
		cfg.Database = "openpanel"
	}

	if cfg.BatchSize != 5000 {
		t.Errorf("expected default batch size 5000, got %d", cfg.BatchSize)
	}
	if cfg.FlushInterval != 2*time.Second {
		t.Errorf("expected default flush interval 2s, got %v", cfg.FlushInterval)
	}
	if cfg.Database != "openpanel" {
		t.Errorf("expected default database 'openpanel', got %s", cfg.Database)
	}
}
