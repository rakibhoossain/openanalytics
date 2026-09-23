package config

import (
	"os"
	"strconv"
)

// Config holds runtime configuration for all services.
type Config struct {
	Env      string
	LogLevel string

	// Server Ports
	IngestPort  string
	QueryPort   string
	MetricsPort string

	// Storage
	DatabaseURL        string // PostgreSQL
	ClickHouseAddr     string // ClickHouse Native TCP
	ClickHouseDatabase string
	ClickHouseUsername string
	ClickHousePassword string

	// Redis
	RedisAddr              string
	RedisPassword          string
	RedisDB                int
	RedisSessionTTLMinutes int

	// Kafka
	KafkaBrokers       string
	KafkaEventsTopic   string
	KafkaSessionsTopic string
	KafkaConsumerGroup string
	KafkaBatchSize     int
	KafkaBatchTimeoutMs int

	// GeoIP (Reads database files populated by geoipupdate docker service)
	GeoIPDataDir string

	// Machine Learning
	MLModelPath        string
	MLInferenceEnabled bool
}

// Load reads configuration from environment variables with defaults.
func Load() *Config {
	return &Config{
		Env:      getEnv("ENV", "development"),
		LogLevel: getEnv("LOG_LEVEL", "info"),

		IngestPort:  getEnv("INGEST_PORT", "8080"),
		QueryPort:   getEnv("QUERY_PORT", "8081"),
		MetricsPort: getEnv("METRICS_PORT", "9090"),

		DatabaseURL:        getEnv("DATABASE_URL", "postgresql://openpanel:openpanel@localhost:5435/openpanel?sslmode=disable"),
		ClickHouseAddr:     getEnv("CLICKHOUSE_ADDR", "127.0.0.1:9000"),
		ClickHouseDatabase: getEnv("CLICKHOUSE_DATABASE", "openpanel"),
		ClickHouseUsername: getEnv("CLICKHOUSE_USERNAME", "openpanel"),
		ClickHousePassword: getEnv("CLICKHOUSE_PASSWORD", "openpanel"),

		RedisAddr:              getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword:          getEnv("REDIS_PASSWORD", ""),
		RedisDB:                getEnvAsInt("REDIS_DB", 0),
		RedisSessionTTLMinutes: getEnvAsInt("REDIS_SESSION_TTL_MINUTES", 30),

		KafkaBrokers:        getEnv("KAFKA_BROKERS", "91.99.83.171:9092"),
		KafkaEventsTopic:    getEnv("KAFKA_EVENTS_TOPIC", "analytics.events.raw"),
		KafkaSessionsTopic:  getEnv("KAFKA_SESSIONS_TOPIC", "analytics.sessions.boundary"),
		KafkaConsumerGroup:  getEnv("KAFKA_CONSUMER_GROUP", "openanalytics-worker-group"),
		KafkaBatchSize:      getEnvAsInt("KAFKA_BATCH_SIZE", 5000),
		KafkaBatchTimeoutMs: getEnvAsInt("KAFKA_BATCH_TIMEOUT_MS", 1000),

		GeoIPDataDir: getEnv("GEOIP_DATA_DIR", "data/geo"),

		MLModelPath:        getEnv("ML_MODEL_PATH", "data/models/cart_intent_v1.onnx"),
		MLInferenceEnabled: getEnvAsBool("ML_INFERENCE_ENABLED", true),
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
	}
	return defaultVal
}
