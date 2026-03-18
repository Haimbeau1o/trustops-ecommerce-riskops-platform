package main

import (
	"context"
	"errors"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/config"
	gatewayhttp "trustops-ecommerce-riskops-platform/services/gateway-go/internal/http"
	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/mq"
	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/storage"
)

func TestBuildRepositoryFailsWhenMySQLRequiredButUnavailable(t *testing.T) {
	prevOpenMySQL := openMySQL
	t.Cleanup(func() {
		openMySQL = prevOpenMySQL
	})

	openMySQL = func(string) (pingCloserDB, error) {
		return nil, errors.New("mysql unavailable")
	}

	_, err := buildRepository(config.Config{
		StorageBackend: "mysql",
		MySQLDSN:       "trustops:trustops@tcp(mysql:3306)/trustops?parseTime=true",
	})
	if err == nil {
		t.Fatalf("expected mysql-backed repository init to fail")
	}
}

func TestBuildRepositoryContinuesWithoutRedisCache(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	mock.ExpectPing()

	prevOpenMySQL := openMySQL
	prevPingRedis := pingRedis
	t.Cleanup(func() {
		openMySQL = prevOpenMySQL
		pingRedis = prevPingRedis
	})

	openMySQL = func(string) (pingCloserDB, error) {
		return db, nil
	}
	pingRedis = func(context.Context, config.Config) error {
		return errors.New("redis unavailable")
	}

	repo, err := buildRepository(config.Config{
		StorageBackend: "mysql",
		MySQLDSN:       "trustops:trustops@tcp(mysql:3306)/trustops?parseTime=true",
		RedisAddr:      "redis:6379",
	})
	if err != nil {
		t.Fatalf("buildRepository() error = %v", err)
	}
	if _, ok := repo.(*storage.InMemoryRepository); ok {
		t.Fatalf("expected mysql-backed repository when redis cache is unavailable")
	}
	if _, ok := repo.(*storage.CachedRepository); ok {
		t.Fatalf("expected direct mysql repository without cache when redis is unavailable")
	}
	if _, ok := repo.(*storage.MySQLRepository); !ok {
		t.Fatalf("expected mysql repository, got %T", repo)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations not met: %v", err)
	}
}

func TestBuildPublisherFailsWhenRabbitMQRequiredButUnavailable(t *testing.T) {
	prevNewRabbitMQPublisher := newRabbitMQPublisher
	t.Cleanup(func() {
		newRabbitMQPublisher = prevNewRabbitMQPublisher
	})

	newRabbitMQPublisher = func(string, string) (mq.Publisher, error) {
		return nil, errors.New("rabbitmq unavailable")
	}

	_, err := buildPublisher(config.Config{
		MQBackend:     "rabbitmq",
		RabbitMQURL:   "amqp://trustops:trustops@rabbitmq:5672/",
		RabbitMQQueue: "risk.case.ingested",
	})
	if err == nil {
		t.Fatalf("expected rabbitmq-backed publisher init to fail")
	}
}

func TestBuildRateLimiterFallsBackToInMemoryWhenRedisUnavailable(t *testing.T) {
	prevPingRedis := pingRedis
	t.Cleanup(func() {
		pingRedis = prevPingRedis
	})

	pingRedis = func(context.Context, config.Config) error {
		return errors.New("redis unavailable")
	}

	limiter := buildRateLimiter(config.Config{
		RedisAddr:       "redis:6379",
		RateLimitRPM:    30,
		RateLimitPrefix: "riskops",
	})
	if limiter == nil {
		t.Fatalf("expected non-nil limiter")
	}
	if _, ok := limiter.(*gatewayhttp.InMemoryRateLimiter); !ok {
		t.Fatalf("expected in-memory limiter fallback, got %T", limiter)
	}
}
