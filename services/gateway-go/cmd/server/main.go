package main

import (
	"context"
	"database/sql"
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

	app := gatewayhttp.NewRouterWithHostPort(cfg.HostPort(), gatewayhttp.Dependencies{
		Repository: repo,
		Publisher:  publisher,
	})
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
