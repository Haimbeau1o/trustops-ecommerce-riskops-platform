package main

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	amqp "github.com/rabbitmq/amqp091-go"

	"trustops-ecommerce-riskops-platform/services/worker/internal/config"
	"trustops-ecommerce-riskops-platform/services/worker/internal/consumer"
	workerstorage "trustops-ecommerce-riskops-platform/services/worker/internal/storage"
)

type pingCloserDB = *sql.DB

var openMySQL = func(dsn string) (pingCloserDB, error) {
	return sql.Open("mysql", dsn)
}

func main() {
	cfg := config.Load()
	logger := log.Default()
	store, err := buildProcessedEventStore(cfg)
	if err != nil {
		log.Fatalf("worker failed to initialize dedup store: %v", err)
	}

	conn, err := amqp.Dial(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("worker failed to connect rabbitmq: %v", err)
	}
	defer conn.Close()

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("worker failed to open channel: %v", err)
	}
	defer ch.Close()

	_, err = ch.QueueDeclare(
		cfg.RabbitMQQueue,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("worker failed to declare queue: %v", err)
	}

	if err := ch.Qos(cfg.Prefetch, 0, false); err != nil {
		log.Fatalf("worker failed to set prefetch: %v", err)
	}

	deliveries, err := ch.Consume(
		cfg.RabbitMQQueue,
		cfg.Name,
		false,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Fatalf("worker failed to start consume: %v", err)
	}

	logger.Printf("worker started name=%s queue=%s", cfg.Name, cfg.RabbitMQQueue)
	for delivery := range deliveries {
		duplicate, err := consumer.ProcessMessage(context.Background(), logger, store, cfg.Name, delivery.Body)
		if err != nil {
			logger.Printf("worker failed to process message: %v", err)
			_ = delivery.Nack(false, false)
			continue
		}
		if duplicate {
			logger.Printf("worker dedup skipped duplicate delivery")
		}
		_ = delivery.Ack(false)
	}
}

func buildProcessedEventStore(cfg config.Config) (workerstorage.ProcessedEventStore, error) {
	if strings.ToLower(cfg.StorageBackend) != "mysql" {
		return workerstorage.NewInMemoryProcessedEventStore(), nil
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
	return workerstorage.NewMySQLProcessedEventStore(db), nil
}
