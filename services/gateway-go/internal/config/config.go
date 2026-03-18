package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains runtime configuration loaded from environment variables.
type Config struct {
	HTTPHost string
	HTTPPort int

	StorageBackend string
	MySQLDSN       string
	RedisAddr      string
	RedisPassword  string
	RedisDB        int
	CaseCacheTTL   time.Duration

	MQBackend     string
	RabbitMQURL   string
	RabbitMQQueue string
}

// Load reads configuration from environment with sensible defaults.
func Load() Config {
	return Config{
		HTTPHost: envOrDefault("GATEWAY_HTTP_HOST", "0.0.0.0"),
		HTTPPort: envIntOrDefault("GATEWAY_HTTP_PORT", 8080),

		StorageBackend: strings.ToLower(envOrDefault("GATEWAY_STORAGE_BACKEND", "memory")),
		MySQLDSN:       envOrDefault("GATEWAY_MYSQL_DSN", "trustops:trustops@tcp(mysql:3306)/trustops?parseTime=true"),
		RedisAddr:      envOrDefault("GATEWAY_REDIS_ADDR", "redis:6379"),
		RedisPassword:  envOrDefault("GATEWAY_REDIS_PASSWORD", ""),
		RedisDB:        envIntOrDefault("GATEWAY_REDIS_DB", 0),
		CaseCacheTTL:   time.Duration(envIntOrDefault("GATEWAY_CASE_CACHE_TTL_SECONDS", 300)) * time.Second,

		MQBackend:     strings.ToLower(envOrDefault("GATEWAY_MQ_BACKEND", "noop")),
		RabbitMQURL:   envOrDefault("GATEWAY_RABBITMQ_URL", "amqp://trustops:trustops@rabbitmq:5672/"),
		RabbitMQQueue: envOrDefault("GATEWAY_RABBITMQ_QUEUE", "risk.case.ingested"),
	}
}

// HostPort returns the HTTP bind address for Hertz.
func (c Config) HostPort() string {
	return fmt.Sprintf("%s:%d", c.HTTPHost, c.HTTPPort)
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
