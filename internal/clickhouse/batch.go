package clickhouse

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"openanalytics/internal/domain"
)

// Config holds connection parameters for ClickHouse.
type Config struct {
	Addr          string
	Database      string
	Username      string
	Password      string
	BatchSize     int
	FlushInterval time.Duration
}

// BatchWriter manages buffering and native columnar TCP block insertion into ClickHouse.
type BatchWriter struct {
	conn          driver.Conn
	database      string
	batchSize     int
	flushInterval time.Duration

	mu             sync.Mutex
	eventsBuffer   []*domain.Event
	sessionsBuffer []*domain.Session
	featuresBuffer []*domain.ShopperFeature
	replayBuffer   []*domain.ReplayChunk

	flushTimer *time.Timer
	isClosed   bool
}

// NewBatchWriter initializes a ClickHouse batch writer.
func NewBatchWriter(ctx context.Context, cfg Config) (*BatchWriter, error) {
	if cfg.BatchSize <= 0 {
		// CRITICAL(clickhouse-block-size): ClickHouse MergeTree requires batches of 1,000 - 10,000+
		// rows per insert. Tiny individual inserts cause "Too many parts" errors and cluster degradation.
		cfg.BatchSize = 5000
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = 2 * time.Second
	}
	if cfg.Database == "" {
		cfg.Database = "openpanel"
	}

	conn, err := clickhouse.Open(&clickhouse.Options{
		Addr: []string{cfg.Addr},
		Auth: clickhouse.Auth{
			Database: cfg.Database,
			Username: cfg.Username,
			Password: cfg.Password,
		},
		Settings: clickhouse.Settings{
			"max_execution_time": 60,
		},
		Compression: &clickhouse.Compression{
			Method: clickhouse.CompressionLZ4,
		},
		DialTimeout: 5 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to open clickhouse connection: %w", err)
	}

	if err := conn.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping clickhouse at %s: %w", cfg.Addr, err)
	}

	w := &BatchWriter{
		conn:           conn,
		database:       cfg.Database,
		batchSize:      cfg.BatchSize,
		flushInterval:  cfg.FlushInterval,
		eventsBuffer:   make([]*domain.Event, 0, cfg.BatchSize),
		sessionsBuffer: make([]*domain.Session, 0, 500),
		featuresBuffer: make([]*domain.ShopperFeature, 0, 500),
		replayBuffer:   make([]*domain.ReplayChunk, 0, 200),
	}

	return w, nil
}

// Conn returns the underlying ClickHouse native driver connection.
func (w *BatchWriter) Conn() driver.Conn {
	return w.conn
}

// AddEvent adds an event to the local buffer, triggering a flush if the batch size is exceeded.
func (w *BatchWriter) AddEvent(ctx context.Context, event *domain.Event) error {
	w.mu.Lock()
	if w.isClosed {
		w.mu.Unlock()
		return fmt.Errorf("batch writer is closed")
	}

	// WEAK_POINT(memory-backpressure): In a prolonged ClickHouse outage, cap maximum memory buffer
	// to avoid OOM killer. Beyond 100,000 events, block or push back on Kafka consumer.
	if len(w.eventsBuffer) >= 100000 {
		w.mu.Unlock()
		return fmt.Errorf("buffer capacity exceeded; pausing consumption for backpressure")
	}

	w.eventsBuffer = append(w.eventsBuffer, event)
	shouldFlush := len(w.eventsBuffer) >= w.batchSize

	if len(w.eventsBuffer) >= 1 && w.flushTimer == nil {
		w.flushTimer = time.AfterFunc(w.flushInterval, func() {
			_ = w.Flush(context.Background())
		})
	}
	w.mu.Unlock()

	if shouldFlush {
		return w.Flush(ctx)
	}
	return nil
}

// AddSession buffers a closed session summary.
func (w *BatchWriter) AddSession(session *domain.Session) {
	if session == nil {
		return
	}
	w.mu.Lock()
	w.sessionsBuffer = append(w.sessionsBuffer, session)
	w.mu.Unlock()
}

// AddShopperFeature buffers an ML feature snapshot for persistence in ClickHouse.
func (w *BatchWriter) AddShopperFeature(feat *domain.ShopperFeature) {
	if feat == nil {
		return
	}
	w.mu.Lock()
	w.featuresBuffer = append(w.featuresBuffer, feat)
	w.mu.Unlock()
}

// AddReplayChunk buffers an rrweb session replay chunk and flushes if capacity reached.
func (w *BatchWriter) AddReplayChunk(ctx context.Context, chunk *domain.ReplayChunk) error {
	if chunk == nil {
		return nil
	}
	w.mu.Lock()
	if w.isClosed {
		w.mu.Unlock()
		return fmt.Errorf("batch writer is closed")
	}
	w.replayBuffer = append(w.replayBuffer, chunk)
	shouldFlush := len(w.replayBuffer) >= 50

	if len(w.replayBuffer) >= 1 && w.flushTimer == nil {
		w.flushTimer = time.AfterFunc(w.flushInterval, func() {
			_ = w.Flush(context.Background())
		})
	}
	w.mu.Unlock()

	if shouldFlush {
		return w.Flush(ctx)
	}
	return nil
}

// Flush writes all buffered events, sessions, ML features, and replay chunks to ClickHouse.
func (w *BatchWriter) Flush(ctx context.Context) error {
	w.mu.Lock()
	if w.flushTimer != nil {
		w.flushTimer.Stop()
		w.flushTimer = nil
	}

	if len(w.eventsBuffer) == 0 && len(w.sessionsBuffer) == 0 && len(w.featuresBuffer) == 0 && len(w.replayBuffer) == 0 {
		w.mu.Unlock()
		return nil
	}

	events := w.eventsBuffer
	sessions := w.sessionsBuffer
	features := w.featuresBuffer
	replays := w.replayBuffer
	w.eventsBuffer = make([]*domain.Event, 0, w.batchSize)
	w.sessionsBuffer = make([]*domain.Session, 0, 500)
	w.featuresBuffer = make([]*domain.ShopperFeature, 0, 500)
	w.replayBuffer = make([]*domain.ReplayChunk, 0, 200)
	w.mu.Unlock()

	// 1. Flush Events
	if len(events) > 0 {
		if err := w.flushEventsWithRetry(ctx, events); err != nil {
			log.Printf("[ClickHouse] ERROR flushing %d events: %v", len(events), err)
			return err
		}
		log.Printf("[ClickHouse] Flushed %d events successfully", len(events))
	}

	// 2. Flush Sessions
	if len(sessions) > 0 {
		if err := w.flushSessionsWithRetry(ctx, sessions); err != nil {
			log.Printf("[ClickHouse] ERROR flushing %d sessions: %v", len(sessions), err)
			return err
		}
		log.Printf("[ClickHouse] Flushed %d sessions successfully", len(sessions))
	}

	// 3. Flush Behavioral ML Features
	if len(features) > 0 {
		if err := w.flushShopperFeaturesWithRetry(ctx, features); err != nil {
			log.Printf("[ClickHouse] ERROR flushing %d shopper features: %v", len(features), err)
		} else {
			log.Printf("[ClickHouse] Flushed %d shopper features successfully", len(features))
		}
	}

	// 4. Flush Session Replay Chunks
	if len(replays) > 0 {
		if err := w.flushReplayChunksWithRetry(ctx, replays); err != nil {
			log.Printf("[ClickHouse] ERROR flushing %d replay chunks: %v", len(replays), err)
			return err
		}
		log.Printf("[ClickHouse] Flushed %d replay chunks successfully", len(replays))
	}

	return nil
}

func (w *BatchWriter) flushEventsWithRetry(ctx context.Context, events []*domain.Event) error {
	query := fmt.Sprintf(`INSERT INTO %s.events (
		id, tenant_id, shop_id, name, device_id, customer_id, session_id,
		revenue, currency, product_id, cart_id, order_id, path, origin,
		referrer, referrer_name, referrer_type, os, browser, device,
		country, city, latitude, longitude, properties, created_at
	)`, w.database)

	batch, err := w.conn.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare events batch: %w", err)
	}

	for _, ev := range events {
		var rev *int64
		if ev.Revenue != nil {
			rev = ev.Revenue
		}

		var lat, lon *float64
		if ev.Latitude != nil {
			v := float64(*ev.Latitude)
			lat = &v
		}
		if ev.Longitude != nil {
			v := float64(*ev.Longitude)
			lon = &v
		}

		props := ev.Properties
		if props == nil {
			props = make(map[string]string)
		}

		err := batch.Append(
			ev.ID,
			ev.TenantID,
			ev.ShopID,
			ev.Name,
			ev.DeviceID,
			ev.CustomerID,
			ev.SessionID,
			rev,
			ev.Currency,
			ev.ProductID,
			ev.CartID,
			ev.OrderID,
			ev.Path,
			ev.Origin,
			ev.Referrer,
			ev.ReferrerName,
			ev.ReferrerType,
			ev.OS,
			ev.Browser,
			ev.Device,
			ev.Country,
			ev.City,
			lat,
			lon,
			props,
			ev.CreatedAt.UTC(),
		)
		if err != nil {
			return fmt.Errorf("failed to append event to batch: %w", err)
		}
	}

	return batch.Send()
}

func (w *BatchWriter) flushSessionsWithRetry(ctx context.Context, sessions []*domain.Session) error {
	query := fmt.Sprintf(`INSERT INTO %s.sessions (
		id, tenant_id, shop_id, device_id, customer_id,
		started_at, ended_at, duration, entry_path, exit_path,
		referrer, referrer_name, referrer_type, events_count,
		has_cart_add, has_purchase, total_revenue
	)`, w.database)

	batch, err := w.conn.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare sessions batch: %w", err)
	}

	for _, s := range sessions {
		var hasCart uint8
		if s.HasCartAdd {
			hasCart = 1
		}
		var hasPurch uint8
		if s.HasPurchase {
			hasPurch = 1
		}

		err := batch.Append(
			s.ID,
			s.TenantID,
			s.ShopID,
			s.DeviceID,
			s.CustomerID,
			s.StartedAt.UTC(),
			s.EndedAt.UTC(),
			s.Duration,
			s.EntryPath,
			s.ExitPath,
			s.Referrer,
			s.ReferrerName,
			s.ReferrerType,
			s.EventsCount,
			hasCart,
			hasPurch,
			s.TotalRevenue,
		)
		if err != nil {
			return fmt.Errorf("failed to append session to batch: %w", err)
		}
	}

	return batch.Send()
}

func (w *BatchWriter) flushShopperFeaturesWithRetry(ctx context.Context, features []*domain.ShopperFeature) error {
	query := fmt.Sprintf(`INSERT INTO %s.shopper_features (
		tenant_id, shop_id, device_id, session_id,
		views_count, cart_adds_count, distinct_products, total_dwell_seconds,
		has_purchase, cart_intent_score, last_event_at
	)`, w.database)

	batch, err := w.conn.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare shopper features batch: %w", err)
	}

	for _, f := range features {
		err := batch.Append(
			f.TenantID,
			f.ShopID,
			f.DeviceID,
			f.SessionID,
			f.ViewsCount,
			f.CartAddsCount,
			f.DistinctProducts,
			f.TotalDwellSeconds,
			f.HasPurchase,
			f.CartIntentScore,
			f.LastEventAt.UTC(),
		)
		if err != nil {
			return fmt.Errorf("failed to append shopper feature to batch: %w", err)
		}
	}

	return batch.Send()
}

func (w *BatchWriter) flushReplayChunksWithRetry(ctx context.Context, chunks []*domain.ReplayChunk) error {
	query := fmt.Sprintf(`INSERT INTO %s.session_replay_chunks (
		tenant_id, shop_id, session_id, chunk_index,
		started_at, ended_at, events_count, is_full_snapshot, payload
	)`, w.database)

	batch, err := w.conn.PrepareBatch(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to prepare replay chunks batch: %w", err)
	}

	for _, c := range chunks {
		var isFullSnapshot uint8
		if c.IsFullSnapshot {
			isFullSnapshot = 1
		}
		err := batch.Append(
			c.TenantID,
			c.ShopID,
			c.SessionID,
			c.ChunkIndex,
			c.StartedAt.UTC(),
			c.EndedAt.UTC(),
			c.EventsCount,
			isFullSnapshot,
			c.Payload,
		)
		if err != nil {
			return fmt.Errorf("failed to append replay chunk to batch: %w", err)
		}
	}

	return batch.Send()
}

// Close flushes all remaining items and closes the ClickHouse connection.
func (w *BatchWriter) Close() error {
	w.mu.Lock()
	w.isClosed = true
	w.mu.Unlock()

	_ = w.Flush(context.Background())
	return w.conn.Close()
}
