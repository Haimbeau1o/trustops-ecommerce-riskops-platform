package consumer

import (
	"bytes"
	"context"
	"log"
	"testing"

	workerstorage "trustops-ecommerce-riskops-platform/services/worker/internal/storage"
)

type fakeProcessedEventStore struct {
	allowStart bool
	processed  []string
	failed     []string
}

func (f *fakeProcessedEventStore) TryStart(_ context.Context, _ string, _ string, _ string) (bool, error) {
	return f.allowStart, nil
}

func (f *fakeProcessedEventStore) MarkProcessed(_ context.Context, eventID string) error {
	f.processed = append(f.processed, eventID)
	return nil
}

func (f *fakeProcessedEventStore) MarkFailed(_ context.Context, eventID string, _ string) error {
	f.failed = append(f.failed, eventID)
	return nil
}

func TestParseCaseIngestedEvent(t *testing.T) {
	input := []byte(`{
		"event_id":"evt-risk-001",
		"case_id":"case-risk-001",
		"merchant_id":"merchant-1001",
		"event_type":"abnormal_listing_activity",
		"risk_score":0.87
	}`)

	event, err := ParseCaseIngestedEvent(input)
	if err != nil {
		t.Fatalf("expected parse success, got: %v", err)
	}
	if event.EventID != "evt-risk-001" {
		t.Fatalf("unexpected event_id %q", event.EventID)
	}
	if event.CaseID != "case-risk-001" {
		t.Fatalf("unexpected case_id %q", event.CaseID)
	}
	if event.MerchantID != "merchant-1001" {
		t.Fatalf("unexpected merchant_id %q", event.MerchantID)
	}
}

func TestParseCaseIngestedEventInvalidJSON(t *testing.T) {
	_, err := ParseCaseIngestedEvent([]byte(`{broken`))
	if err == nil {
		t.Fatalf("expected parse failure for invalid json")
	}
}

func TestProcessMessageSkipsDuplicateEvent(t *testing.T) {
	store := &fakeProcessedEventStore{allowStart: false}
	logger := log.New(bytes.NewBuffer(nil), "", 0)
	body := []byte(`{
		"event_id":"evt-risk-001",
		"case_id":"case-risk-001",
		"merchant_id":"merchant-1001",
		"event_type":"abnormal_listing_activity",
		"risk_score":0.87
	}`)

	duplicate, err := ProcessMessage(context.Background(), logger, store, "worker-a", body)
	if err != nil {
		t.Fatalf("expected duplicate skip without error, got %v", err)
	}
	if !duplicate {
		t.Fatalf("expected duplicate=true")
	}
	if len(store.processed) != 0 {
		t.Fatalf("expected no processed markers, got %#v", store.processed)
	}
}

func TestProcessMessageMarksProcessedOnSuccess(t *testing.T) {
	store := &fakeProcessedEventStore{allowStart: true}
	logger := log.New(bytes.NewBuffer(nil), "", 0)
	body := []byte(`{
		"event_id":"evt-risk-001",
		"case_id":"case-risk-001",
		"merchant_id":"merchant-1001",
		"event_type":"abnormal_listing_activity",
		"risk_score":0.87
	}`)

	duplicate, err := ProcessMessage(context.Background(), logger, store, "worker-a", body)
	if err != nil {
		t.Fatalf("expected process success, got %v", err)
	}
	if duplicate {
		t.Fatalf("expected duplicate=false")
	}
	if len(store.processed) != 1 || store.processed[0] != "evt-risk-001" {
		t.Fatalf("unexpected processed events %#v", store.processed)
	}
}

var _ workerstorage.ProcessedEventStore = (*fakeProcessedEventStore)(nil)
