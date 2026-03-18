package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"

	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/config"
	gatewayhttp "trustops-ecommerce-riskops-platform/services/gateway-go/internal/http"
	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/mq"
	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/storage"
)

type pingCloserDB = *sql.DB

var openMySQL = func(dsn string) (pingCloserDB, error) {
	return sql.Open("mysql", dsn)
}

var pingRedis = func(ctx context.Context, cfg config.Config) error {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer redisClient.Close()
	return redisClient.Ping(ctx).Err()
}

var newRedisCacheClient = func(cfg config.Config) *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
}

var newRabbitMQPublisher = func(url, queue string) (mq.Publisher, error) {
	return mq.NewRabbitMQPublisher(url, queue)
}

const (
	outboxRelayBatchSize  = 20
	outboxRelayMaxRetries = 3
)

func main() {
	cfg := config.Load()
	repo, err := buildRepository(cfg)
	if err != nil {
		log.Fatalf("gateway repository init failed: %v", err)
	}
	publisher, err := buildPublisher(cfg)
	if err != nil {
		log.Fatalf("gateway publisher init failed: %v", err)
	}
	runtimeMetrics := gatewayhttp.NewRuntimeMetrics()

	app := gatewayhttp.NewRouterWithHostPort(cfg.HostPort(), gatewayhttp.Dependencies{
		Repository:     repo,
		Authenticator:  buildAuthenticator(cfg),
		RateLimiter:    buildRateLimiter(cfg),
		RuntimeMetrics: runtimeMetrics,
	})
	startOutboxRelay(repo, publisher)
	log.Printf("gateway-go listening on %s with storage=%s mq=%s", cfg.HostPort(), cfg.StorageBackend, cfg.MQBackend)
	app.Spin()
}

func buildRepository(cfg config.Config) (storage.Repository, error) {
	if cfg.StorageBackend != "mysql" {
		return storage.NewInMemoryRepository(), nil
	}

	db, err := openMySQL(cfg.MySQLDSN)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	baseRepo := storage.NewMySQLRepository(db)

	if err := pingRedis(ctx, cfg); err != nil {
		log.Printf("failed to ping redis, continue without cache: %v", err)
		return baseRepo, nil
	}

	redisClient := newRedisCacheClient(cfg)
	cache := storage.NewRedisCaseCache(redisClient)
	return storage.NewCachedRepository(baseRepo, cache, cfg.CaseCacheTTL), nil
}

func buildPublisher(cfg config.Config) (mq.Publisher, error) {
	if cfg.MQBackend != "rabbitmq" {
		return mq.NewNoopPublisher(), nil
	}

	publisher, err := newRabbitMQPublisher(cfg.RabbitMQURL, cfg.RabbitMQQueue)
	if err != nil {
		return nil, err
	}
	return publisher, nil
}

func buildAuthenticator(cfg config.Config) gatewayhttp.Authenticator {
	return gatewayhttp.NewAPIKeyAuthenticator(cfg.APIKeys)
}

func buildRateLimiter(cfg config.Config) gatewayhttp.RateLimiter {
	if cfg.RateLimitRPM <= 0 {
		return gatewayhttp.NewNoopRateLimiter()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := pingRedis(ctx, cfg); err != nil {
		log.Printf("failed to ping redis for rate limiting, continue with in-memory limiter: %v", err)
		return gatewayhttp.NewInMemoryRateLimiter(cfg.RateLimitRPM)
	}

	return gatewayhttp.NewRedisRateLimiter(newRedisCacheClient(cfg), cfg.RateLimitPrefix, cfg.RateLimitRPM)
}

func startOutboxRelay(repo storage.Repository, publisher mq.Publisher) {
	ticker := time.NewTicker(2 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			flushCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			if err := flushOutbox(flushCtx, repo, publisher, time.Now().UTC()); err != nil {
				log.Printf("outbox relay flush failed: %v", err)
			}
			cancel()
			<-ticker.C
		}
	}()
}

func flushOutbox(ctx context.Context, repo storage.Repository, publisher mq.Publisher, now time.Time) error {
	items, err := repo.ListPendingOutbox(ctx, now, outboxRelayBatchSize)
	if err != nil {
		return err
	}

	for _, item := range items {
		var event mq.CaseIngestedEvent
		if err := json.Unmarshal(item.Payload, &event); err != nil {
			if _, markErr := repo.MarkOutboxFailed(ctx, item, "invalid_outbox_payload", now, 1); markErr != nil {
				return markErr
			}
			continue
		}

		if err := publisher.PublishCaseIngested(ctx, event); err != nil {
			nextAttemptAt := now.Add(time.Duration(item.PublishAttempts+1) * 5 * time.Second)
			if _, markErr := repo.MarkOutboxFailed(ctx, item, err.Error(), nextAttemptAt, outboxRelayMaxRetries); markErr != nil {
				return markErr
			}
			continue
		}

		if err := repo.MarkOutboxPublished(ctx, item.ID); err != nil {
			return err
		}
	}

	return nil
}
