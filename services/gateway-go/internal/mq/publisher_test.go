package mq

import (
	"context"
	"encoding/json"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

type fakeAMQPChannel struct {
	declaredQueue string
	declareCalls  []struct {
		name string
		args amqp.Table
	}
	published   []amqp.Publishing
	routingKeys []string
}

func (f *fakeAMQPChannel) QueueDeclare(
	name string,
	_ bool,
	_ bool,
	_ bool,
	_ bool,
	args amqp.Table,
) (amqp.Queue, error) {
	f.declaredQueue = name
	f.declareCalls = append(f.declareCalls, struct {
		name string
		args amqp.Table
	}{name: name, args: args})
	return amqp.Queue{Name: name}, nil
}

func (f *fakeAMQPChannel) PublishWithContext(
	_ context.Context,
	_ string,
	key string,
	_ bool,
	_ bool,
	msg amqp.Publishing,
) error {
	f.routingKeys = append(f.routingKeys, key)
	f.published = append(f.published, msg)
	return nil
}

func TestNoopPublisher(t *testing.T) {
	publisher := NewNoopPublisher()

	err := publisher.PublishCaseIngested(context.Background(), CaseIngestedEvent{
		CaseID:     "case-risk-001",
		MerchantID: "merchant-1001",
		EventType:  "abnormal_listing_activity",
		RiskScore:  0.87,
	})
	if err != nil {
		t.Fatalf("noop publisher should not return error, got: %v", err)
	}
}

func TestRabbitMQPublisherPublishesCaseEvent(t *testing.T) {
	ch := &fakeAMQPChannel{}
	publisher, err := NewRabbitMQPublisherFromChannel(ch, "risk.case.ingested")
	if err != nil {
		t.Fatalf("failed to create publisher: %v", err)
	}

	input := CaseIngestedEvent{
		CaseID:     "case-risk-001",
		MerchantID: "merchant-1001",
		EventType:  "abnormal_listing_activity",
		RiskScore:  0.87,
	}
	if err := publisher.PublishCaseIngested(context.Background(), input); err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	if len(ch.declareCalls) != 2 {
		t.Fatalf("expected main queue and dlq declarations, got %d", len(ch.declareCalls))
	}
	if ch.declareCalls[0].name != "risk.case.ingested" {
		t.Fatalf("expected main queue declaration first, got %#v", ch.declareCalls[0])
	}
	if ch.declareCalls[0].args["x-dead-letter-routing-key"] != "risk.case.ingested.dlq" {
		t.Fatalf("expected main queue dead-letter routing key, got %#v", ch.declareCalls[0].args)
	}
	if ch.declareCalls[1].name != "risk.case.ingested.dlq" {
		t.Fatalf("expected dlq declaration, got %#v", ch.declareCalls[1])
	}
	if len(ch.published) != 1 {
		t.Fatalf("expected one message to be published, got %d", len(ch.published))
	}
	if len(ch.routingKeys) != 1 || ch.routingKeys[0] != "risk.case.ingested" {
		t.Fatalf("unexpected routing keys: %#v", ch.routingKeys)
	}

	var payload CaseIngestedEvent
	if err := json.Unmarshal(ch.published[0].Body, &payload); err != nil {
		t.Fatalf("failed to unmarshal published body: %v", err)
	}
	if payload.CaseID != input.CaseID || payload.MerchantID != input.MerchantID {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}
