package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"log"

	workerstorage "trustops-ecommerce-riskops-platform/services/worker/internal/storage"
)

var ErrInvalidCaseEvent = errors.New("invalid_case_ingested_event")

// CaseIngestedEvent mirrors gateway's lightweight queue event.
type CaseIngestedEvent struct {
	EventID      string  `json:"event_id"`
	CaseID       string  `json:"case_id"`
	MerchantID   string  `json:"merchant_id"`
	EventType    string  `json:"event_type"`
	RiskScore    float64 `json:"risk_score"`
	OccurredAtMs int64   `json:"occurred_at_ms,omitempty"`
}

func ParseCaseIngestedEvent(body []byte) (CaseIngestedEvent, error) {
	var event CaseIngestedEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return CaseIngestedEvent{}, err
	}
	if event.EventID == "" || event.CaseID == "" || event.MerchantID == "" || event.EventType == "" {
		return CaseIngestedEvent{}, ErrInvalidCaseEvent
	}
	return event, nil
}

type Logger interface {
	Printf(format string, v ...any)
}

// ProcessMessage parses a queue message, applies persistent dedup, and logs placeholder business logic.
func ProcessMessage(ctx context.Context, logger Logger, store workerstorage.ProcessedEventStore, consumerName string, body []byte) (bool, error) {
	event, err := ParseCaseIngestedEvent(body)
	if err != nil {
		return false, err
	}

	if store != nil {
		acquired, err := store.TryStart(ctx, event.EventID, event.CaseID, consumerName)
		if err != nil {
			return false, err
		}
		if !acquired {
			logger.Printf("skipped duplicate case event event_id=%s case_id=%s", event.EventID, event.CaseID)
			return true, nil
		}
	}

	processCaseEvent(logger, event)
	if store != nil {
		if err := store.MarkProcessed(ctx, event.EventID); err != nil {
			_ = store.MarkFailed(ctx, event.EventID, err.Error())
			return false, err
		}
	}
	return false, nil
}

func processCaseEvent(logger Logger, event CaseIngestedEvent) {
	logger.Printf(
		"processed case event event_id=%s case_id=%s merchant_id=%s event_type=%s risk_score=%.4f",
		event.EventID,
		event.CaseID,
		event.MerchantID,
		event.EventType,
		event.RiskScore,
	)
}

var _ Logger = (*log.Logger)(nil)
