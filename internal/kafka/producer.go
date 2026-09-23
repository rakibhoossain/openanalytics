package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"openanalytics/internal/domain"
)

// ProducerConfig contains settings for the Kafka partitioned producer.
type ProducerConfig struct {
	Brokers        string
	Topic          string
	BatchSize      int
	BatchTimeoutMs int
}

// Producer produces events into Kafka with partition affinity.
type Producer struct {
	writer *kafka.Writer
	topic  string
}

// NewProducer creates a new high-throughput partitioned Kafka writer.
func NewProducer(cfg ProducerConfig) *Producer {
	brokers := strings.Split(cfg.Brokers, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 1000
	}

	batchTimeout := time.Duration(cfg.BatchTimeoutMs) * time.Millisecond
	if batchTimeout <= 0 {
		batchTimeout = 50 * time.Millisecond
	}

	writer := &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        cfg.Topic,
		Balancer:     &kafka.Hash{}, // Guarantees identical key (shop_id:device_id) routes to identical partition
		Compression:  kafka.Snappy,
		BatchSize:    batchSize,
		BatchTimeout: batchTimeout,
		Async:        true, // Non-blocking async dispatch for maximum throughput
		RequiredAcks: kafka.RequireOne,
	}

	return &Producer{
		writer: writer,
		topic:  cfg.Topic,
	}
}

// Produce sends a single event to Kafka, keyed by shop_id:device_id.
func (p *Producer) Produce(ctx context.Context, event *domain.Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	key := fmt.Sprintf("%s:%s", event.ShopID.String(), event.DeviceID)

	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(key),
		Value: data,
		Time:  event.CreatedAt,
	})
}

// ProduceBatch sends multiple events in a single batch to Kafka.
func (p *Producer) ProduceBatch(ctx context.Context, events []*domain.Event) error {
	if len(events) == 0 {
		return nil
	}

	msgs := make([]kafka.Message, len(events))
	for i, ev := range events {
		data, err := json.Marshal(ev)
		if err != nil {
			return fmt.Errorf("failed to marshal event at index %d: %w", i, err)
		}
		key := fmt.Sprintf("%s:%s", ev.ShopID.String(), ev.DeviceID)
		msgs[i] = kafka.Message{
			Key:   []byte(key),
			Value: data,
			Time:  ev.CreatedAt,
		}
	}

	return p.writer.WriteMessages(ctx, msgs...)
}

// Close flushes in-flight events and closes the writer.
func (p *Producer) Close() error {
	return p.writer.Close()
}
