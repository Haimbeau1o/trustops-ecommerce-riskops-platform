package main

import (
	"log"

	amqp "github.com/rabbitmq/amqp091-go"

	"trustops-ecommerce-riskops-platform/services/worker/internal/config"
	"trustops-ecommerce-riskops-platform/services/worker/internal/consumer"
)

func main() {
	cfg := config.Load()
	logger := log.Default()

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
		if err := consumer.ProcessMessage(logger, delivery.Body); err != nil {
			logger.Printf("worker failed to process message: %v", err)
			_ = delivery.Nack(false, false)
			continue
		}
		_ = delivery.Ack(false)
	}
}
