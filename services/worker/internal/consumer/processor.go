package consumer

import (
	"encoding/json"
	"errors"
	"log"
)

var ErrInvalidCaseEvent = errors.New("invalid_case_ingested_event")

// CaseIngestedEvent mirrors gateway's lightweight queue event.
type CaseIngestedEvent struct {
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
	if event.CaseID == "" || event.MerchantID == "" || event.EventType == "" {
		return CaseIngestedEvent{}, ErrInvalidCaseEvent
	}
	return event, nil
}

// ProcessMessage parses and logs event fields as placeholder business logic.
func ProcessMessage(logger *log.Logger, body []byte) error {
	event, err := ParseCaseIngestedEvent(body)
	if err != nil {
		return err
	}
	logger.Printf(
		"processed case event case_id=%s merchant_id=%s event_type=%s risk_score=%.4f",
		event.CaseID,
		event.MerchantID,
		event.EventType,
		event.RiskScore,
	)
	return nil
}
