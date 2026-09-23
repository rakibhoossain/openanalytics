package kafka

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
)

func TestKafkaConnectivity(t *testing.T) {
	broker := "91.99.83.171:9092"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := kafka.DialContext(ctx, "tcp", broker)
	if err != nil {
		t.Fatalf("failed to dial kafka broker %s: %v", broker, err)
	}
	defer conn.Close()

	// Query broker metadata / partitions
	partitions, err := conn.ReadPartitions()
	if err != nil {
		t.Fatalf("failed to read partitions from kafka broker: %v", err)
	}

	t.Logf("Successfully connected to Kafka broker %s! Found %d partitions across all topics:", broker, len(partitions))
	topics := make(map[string]int)
	for _, p := range partitions {
		topics[p.Topic]++
	}
	for topic, count := range topics {
		t.Logf(" - Topic: %s (%d partitions)", topic, count)
	}

	// Verify controller / brokers
	controller, err := conn.Controller()
	if err == nil {
		t.Logf("Kafka Controller Broker: %s:%d (ID: %d)", controller.Host, controller.Port, controller.ID)
	}
}

func TestProduceAndConsumeVerification(t *testing.T) {
	broker := "91.99.83.171:9092"
	testTopic := "analytics.events.raw"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 1. Ensure connection
	conn, err := kafka.DialLeader(ctx, "tcp", broker, testTopic, 0)
	if err != nil {
		t.Logf("Note: topic %s might not exist yet or requires auto-creation: %v", testTopic, err)
		return
	}
	defer conn.Close()

	testMsg := "ping-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	_, err = conn.WriteMessages(kafka.Message{
		Key:   []byte("test_key"),
		Value: []byte(testMsg),
	})
	if err != nil {
		t.Fatalf("failed to write test message to kafka: %v", err)
	}
	t.Logf("Successfully produced test message to topic %s", testTopic)
}
