package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_HOST", "")
	t.Setenv("GATEWAY_HTTP_PORT", "")
	t.Setenv("GATEWAY_STORAGE_BACKEND", "")
	t.Setenv("GATEWAY_MYSQL_DSN", "")
	t.Setenv("GATEWAY_REDIS_ADDR", "")
	t.Setenv("GATEWAY_REDIS_PASSWORD", "")
	t.Setenv("GATEWAY_REDIS_DB", "")
	t.Setenv("GATEWAY_CASE_CACHE_TTL_SECONDS", "")
	t.Setenv("GATEWAY_MQ_BACKEND", "")
	t.Setenv("GATEWAY_RABBITMQ_URL", "")
	t.Setenv("GATEWAY_RABBITMQ_QUEUE", "")
	t.Setenv("GATEWAY_API_KEYS", "")
	t.Setenv("GATEWAY_RATE_LIMIT_RPM", "")
	t.Setenv("GATEWAY_RATE_LIMIT_PREFIX", "")

	cfg := Load()

	if cfg.HTTPHost != "0.0.0.0" {
		t.Fatalf("expected default host 0.0.0.0, got %q", cfg.HTTPHost)
	}
	if cfg.HTTPPort != 8080 {
		t.Fatalf("expected default port 8080, got %d", cfg.HTTPPort)
	}
	if cfg.StorageBackend != "memory" {
		t.Fatalf("expected default storage backend memory, got %q", cfg.StorageBackend)
	}
	if cfg.MySQLDSN == "" {
		t.Fatalf("expected default mysql dsn to be non-empty")
	}
	if cfg.RedisAddr != "redis:6379" {
		t.Fatalf("expected default redis addr redis:6379, got %q", cfg.RedisAddr)
	}
	if cfg.RedisDB != 0 {
		t.Fatalf("expected default redis db 0, got %d", cfg.RedisDB)
	}
	if cfg.CaseCacheTTL != 300*time.Second {
		t.Fatalf("expected default cache ttl 300s, got %s", cfg.CaseCacheTTL)
	}
	if cfg.MQBackend != "noop" {
		t.Fatalf("expected default mq backend noop, got %q", cfg.MQBackend)
	}
	if cfg.RabbitMQURL != "amqp://trustops:trustops@rabbitmq:5672/" {
		t.Fatalf("expected default rabbitmq url amqp://trustops:trustops@rabbitmq:5672/, got %q", cfg.RabbitMQURL)
	}
	if cfg.RabbitMQQueue != "risk.case.ingested" {
		t.Fatalf("expected default queue risk.case.ingested, got %q", cfg.RabbitMQQueue)
	}
	if len(cfg.APIKeys) != 1 || cfg.APIKeys[0] != "riskops-dev-key" {
		t.Fatalf("expected default api keys [riskops-dev-key], got %#v", cfg.APIKeys)
	}
	if cfg.RateLimitRPM != 60 {
		t.Fatalf("expected default rate limit rpm 60, got %d", cfg.RateLimitRPM)
	}
	if cfg.RateLimitPrefix != "riskops" {
		t.Fatalf("expected default rate limit prefix riskops, got %q", cfg.RateLimitPrefix)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("GATEWAY_HTTP_HOST", "127.0.0.1")
	t.Setenv("GATEWAY_HTTP_PORT", "9090")
	t.Setenv("GATEWAY_STORAGE_BACKEND", "mysql")
	t.Setenv("GATEWAY_MYSQL_DSN", "foo:bar@tcp(localhost:3306)/risk")
	t.Setenv("GATEWAY_REDIS_ADDR", "127.0.0.1:6380")
	t.Setenv("GATEWAY_REDIS_PASSWORD", "secret")
	t.Setenv("GATEWAY_REDIS_DB", "2")
	t.Setenv("GATEWAY_CASE_CACHE_TTL_SECONDS", "45")
	t.Setenv("GATEWAY_MQ_BACKEND", "rabbitmq")
	t.Setenv("GATEWAY_RABBITMQ_URL", "amqp://trustops:trustops@localhost:5672/")
	t.Setenv("GATEWAY_RABBITMQ_QUEUE", "risk.ops.case.events")
	t.Setenv("GATEWAY_API_KEYS", "ops-a,ops-b")
	t.Setenv("GATEWAY_RATE_LIMIT_RPM", "120")
	t.Setenv("GATEWAY_RATE_LIMIT_PREFIX", "riskops-prod")

	cfg := Load()

	if cfg.HTTPHost != "127.0.0.1" {
		t.Fatalf("expected host 127.0.0.1, got %q", cfg.HTTPHost)
	}
	if cfg.HTTPPort != 9090 {
		t.Fatalf("expected port 9090, got %d", cfg.HTTPPort)
	}
	if cfg.StorageBackend != "mysql" {
		t.Fatalf("expected storage backend mysql, got %q", cfg.StorageBackend)
	}
	if cfg.MySQLDSN != "foo:bar@tcp(localhost:3306)/risk" {
		t.Fatalf("unexpected mysql dsn %q", cfg.MySQLDSN)
	}
	if cfg.RedisAddr != "127.0.0.1:6380" {
		t.Fatalf("unexpected redis addr %q", cfg.RedisAddr)
	}
	if cfg.RedisPassword != "secret" {
		t.Fatalf("unexpected redis password %q", cfg.RedisPassword)
	}
	if cfg.RedisDB != 2 {
		t.Fatalf("unexpected redis db %d", cfg.RedisDB)
	}
	if cfg.CaseCacheTTL != 45*time.Second {
		t.Fatalf("unexpected cache ttl %s", cfg.CaseCacheTTL)
	}
	if cfg.MQBackend != "rabbitmq" {
		t.Fatalf("unexpected mq backend %q", cfg.MQBackend)
	}
	if cfg.RabbitMQURL != "amqp://trustops:trustops@localhost:5672/" {
		t.Fatalf("unexpected rabbitmq url %q", cfg.RabbitMQURL)
	}
	if cfg.RabbitMQQueue != "risk.ops.case.events" {
		t.Fatalf("unexpected rabbitmq queue %q", cfg.RabbitMQQueue)
	}
	if len(cfg.APIKeys) != 2 || cfg.APIKeys[0] != "ops-a" || cfg.APIKeys[1] != "ops-b" {
		t.Fatalf("unexpected api keys %#v", cfg.APIKeys)
	}
	if cfg.RateLimitRPM != 120 {
		t.Fatalf("unexpected rate limit rpm %d", cfg.RateLimitRPM)
	}
	if cfg.RateLimitPrefix != "riskops-prod" {
		t.Fatalf("unexpected rate limit prefix %q", cfg.RateLimitPrefix)
	}
}
