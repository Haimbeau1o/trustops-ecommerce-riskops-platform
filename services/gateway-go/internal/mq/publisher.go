package mq

import (
	"context"
	"encoding/json"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// CaseIngestedEvent is a lightweight event emitted after a case is ingested.
type CaseIngestedEvent struct {
	EventID      string  `json:"event_id"`
	CaseID       string  `json:"case_id"`
	MerchantID   string  `json:"merchant_id"`
	EventType    string  `json:"event_type"`
	RiskScore    float64 `json:"risk_score"`
	OccurredAtMs int64   `json:"occurred_at_ms,omitempty"`
}

// Publisher abstracts queue publishing behavior.
type Publisher interface {
	PublishCaseIngested(ctx context.Context, event CaseIngestedEvent) error
}

type noopPublisher struct{}

func NewNoopPublisher() Publisher {
	return &noopPublisher{}
}

func (p *noopPublisher) PublishCaseIngested(_ context.Context, _ CaseIngestedEvent) error {
	return nil
}

type amqpChannel interface {
	QueueDeclare(name string, durable, autoDelete, exclusive, noWait bool, args amqp.Table) (amqp.Queue, error)
	PublishWithContext(ctx context.Context, exchange, key string, mandatory, immediate bool, msg amqp.Publishing) error
}

// RabbitMQPublisher publishes events to RabbitMQ.
type RabbitMQPublisher struct {
	channel amqpChannel
	queue   string
}

func NewRabbitMQPublisher(url, queue string) (*RabbitMQPublisher, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, err
	}
	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return NewRabbitMQPublisherFromChannel(ch, queue)
}

func NewRabbitMQPublisherFromChannel(channel amqpChannel, queue string) (*RabbitMQPublisher, error) {
	_, err := channel.QueueDeclare(
		queue,
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return nil, err
	}
	return &RabbitMQPublisher{
		channel: channel,
		queue:   queue,
	}, nil
}

func (p *RabbitMQPublisher) PublishCaseIngested(ctx context.Context, event CaseIngestedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return p.channel.PublishWithContext(
		ctx,
		"",
		p.queue,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			Body:         body,
			DeliveryMode: amqp.Persistent,
			Timestamp:    time.Now().UTC(),
		},
	)
}
