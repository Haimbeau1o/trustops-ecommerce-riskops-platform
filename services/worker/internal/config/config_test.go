package config

import "testing"

func TestLoadDefaults(t *testing.T) {
	t.Setenv("WORKER_NAME", "")
	t.Setenv("WORKER_RABBITMQ_URL", "")
	t.Setenv("WORKER_RABBITMQ_QUEUE", "")
	t.Setenv("WORKER_PREFETCH", "")
	t.Setenv("WORKER_STORAGE_BACKEND", "")
	t.Setenv("WORKER_MYSQL_DSN", "")

	cfg := Load()

	if cfg.Name != "riskops-worker" {
		t.Fatalf("expected default worker name riskops-worker, got %q", cfg.Name)
	}
	if cfg.RabbitMQURL != "amqp://trustops:trustops@rabbitmq:5672/" {
		t.Fatalf("expected default rabbitmq url amqp://trustops:trustops@rabbitmq:5672/, got %q", cfg.RabbitMQURL)
	}
	if cfg.RabbitMQQueue != "risk.case.ingested" {
		t.Fatalf("expected default queue risk.case.ingested, got %q", cfg.RabbitMQQueue)
	}
	if cfg.Prefetch != 10 {
		t.Fatalf("expected default prefetch 10, got %d", cfg.Prefetch)
	}
	if cfg.StorageBackend != "memory" {
		t.Fatalf("expected default storage backend memory, got %q", cfg.StorageBackend)
	}
	if cfg.MySQLDSN != "trustops:trustops@tcp(mysql:3306)/trustops?parseTime=true" {
		t.Fatalf("unexpected default mysql dsn %q", cfg.MySQLDSN)
	}
}

func TestLoadFromEnv(t *testing.T) {
	t.Setenv("WORKER_NAME", "worker-a")
	t.Setenv("WORKER_RABBITMQ_URL", "amqp://trustops:trustops@localhost:5672/")
	t.Setenv("WORKER_RABBITMQ_QUEUE", "risk.ops.case.events")
	t.Setenv("WORKER_PREFETCH", "5")
	t.Setenv("WORKER_STORAGE_BACKEND", "mysql")
	t.Setenv("WORKER_MYSQL_DSN", "foo:bar@tcp(localhost:3306)/risk")

	cfg := Load()

	if cfg.Name != "worker-a" {
		t.Fatalf("expected worker name worker-a, got %q", cfg.Name)
	}
	if cfg.RabbitMQURL != "amqp://trustops:trustops@localhost:5672/" {
		t.Fatalf("unexpected rabbitmq url %q", cfg.RabbitMQURL)
	}
	if cfg.RabbitMQQueue != "risk.ops.case.events" {
		t.Fatalf("unexpected rabbitmq queue %q", cfg.RabbitMQQueue)
	}
	if cfg.Prefetch != 5 {
		t.Fatalf("expected prefetch 5, got %d", cfg.Prefetch)
	}
	if cfg.StorageBackend != "mysql" {
		t.Fatalf("expected storage backend mysql, got %q", cfg.StorageBackend)
	}
	if cfg.MySQLDSN != "foo:bar@tcp(localhost:3306)/risk" {
		t.Fatalf("unexpected mysql dsn %q", cfg.MySQLDSN)
	}
}
