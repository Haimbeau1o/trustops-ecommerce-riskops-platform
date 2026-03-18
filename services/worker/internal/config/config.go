package config

import (
	"os"
	"strconv"
	"strings"
)

// Config contains worker runtime configuration from environment.
type Config struct {
	Name          string
	RabbitMQURL   string
	RabbitMQQueue string
	Prefetch      int
}

// Load parses worker config with defaults suitable for local docker compose.
func Load() Config {
	return Config{
		Name:          envOrDefault("WORKER_NAME", "riskops-worker"),
		RabbitMQURL:   envOrDefault("WORKER_RABBITMQ_URL", "amqp://trustops:trustops@rabbitmq:5672/"),
		RabbitMQQueue: envOrDefault("WORKER_RABBITMQ_QUEUE", "risk.case.ingested"),
		Prefetch:      envIntOrDefault("WORKER_PREFETCH", 10),
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
