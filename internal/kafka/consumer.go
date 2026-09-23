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
		MinBytes:       1,               // 1 byte for instant event consumption
		MaxBytes:       10e6,            // 10MB
		MaxWait:        2 * time.Second, // Allow WAN roundtrip latency
		CommitInterval: 500 * time.Millisecond,
		StartOffset:    kafka.FirstOffset,
		ErrorLogger:    kafka.LoggerFunc(log.Printf),
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
			// Read next message from assigned partition
			msg, err := c.reader.ReadMessage(ctx)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				log.Printf("[Worker %s] Read error: %v (backing off 500ms)", c.workerID, err)
				time.Sleep(500 * time.Millisecond)
				continue
			}

			var event domain.Event
			if err := json.Unmarshal(msg.Value, &event); err != nil {
				// WEAK_POINT(malformed-event): If an event cannot be deserialized, log to prevent poison pills
				log.Printf("[Worker %s] Poison pill dropped: failed to unmarshal message at offset %d: %v", c.workerID, msg.Offset, err)
				continue
			}

			// Process event through session state machine and buffer to ClickHouse
			if err := handler(ctx, &event); err != nil {
				log.Printf("[Worker %s] Handler error at partition %d offset %d: %v", c.workerID, msg.Partition, msg.Offset, err)
				time.Sleep(200 * time.Millisecond)
				continue
			}
		}
	}
}

// Close closes the Kafka reader connection.
func (c *Consumer) Close() error {
	return c.reader.Close()
}
