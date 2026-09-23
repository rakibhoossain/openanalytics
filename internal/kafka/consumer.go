package kafka

import (
	"context"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"openanalytics/internal/domain"
)

// ConsumerConfig contains parameters for the Kafka Consumer Group.
type ConsumerConfig struct {
	Brokers       string
	Topic         string
	ConsumerGroup string
	WorkerID      string
}

// Consumer consumes partitioned event streams in a coordinated consumer group.
type Consumer struct {
	reader   *kafka.Reader
	workerID string
}

// NewConsumer creates a new Kafka Consumer Group reader.
func NewConsumer(cfg ConsumerConfig) *Consumer {
	brokers := strings.Split(cfg.Brokers, ",")
	for i := range brokers {
		brokers[i] = strings.TrimSpace(brokers[i])
	}

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          cfg.Topic,
		GroupID:        cfg.ConsumerGroup,
		MinBytes:       10e3,            // 10KB
		MaxBytes:       10e6,            // 10MB
		MaxWait:        200 * time.Millisecond,
		CommitInterval: 1 * time.Second, // Async offset commit interval
		StartOffset:    kafka.LastOffset,
	})

	return &Consumer{
		reader:   reader,
		workerID: cfg.WorkerID,
	}
}

// ConsumeLoop listens to Kafka and invokes the processor callback for each incoming event.
func (c *Consumer) ConsumeLoop(ctx context.Context, handler func(ctx context.Context, event *domain.Event) error) error {
	log.Printf("[Worker %s] Started Kafka consumer loop for topic %s", c.workerID, c.reader.Config().Topic)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Worker %s] Exiting consumer loop...", c.workerID)
			return nil
		default:
			// Fetch next message from assigned partition
			msg, err := c.reader.FetchMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("[Worker %s] Fetch error: %v (backing off 500ms)", c.workerID, err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			var event domain.Event
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				// WEAK_POINT(malformed-event): If an event cannot be deserialized, log and commit
				// to prevent poison pills from permanently stalling partition consumption.
				log.Printf("[Worker %s] Poison pill dropped: failed to unmarshal message at offset %d: %v", c.workerID, msg.Offset, err)
				_ = c.reader.CommitMessages(ctx, msg)
				continue
			}

			// Process event through session state machine and buffer to ClickHouse
			if err := handler(ctx, &event); err != nil {
				log.Printf("[Worker %s] Handler error at partition %d offset %d: %v", c.workerID, msg.Partition, msg.Offset, err)
				// Backoff on downstream failure
				time.Sleep(200 * time.Millisecond)
				continue
			}

			// CRITICAL(at-least-once): Commit offset only after successful processing.
			if err := c.reader.CommitMessages(ctx, msg); err != nil {
				log.Printf("[Worker %s] Warning: offset commit error: %v", c.workerID, err)
			}
		}
	}
}

// Close closes the Kafka reader connection.
func (c *Consumer) Close() error {
	return c.reader.Close()
}
