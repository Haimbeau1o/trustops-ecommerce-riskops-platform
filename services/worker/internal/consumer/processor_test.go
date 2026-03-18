package consumer

import "testing"

func TestParseCaseIngestedEvent(t *testing.T) {
	input := []byte(`{
		"case_id":"case-risk-001",
		"merchant_id":"merchant-1001",
		"event_type":"abnormal_listing_activity",
		"risk_score":0.87
	}`)

	event, err := ParseCaseIngestedEvent(input)
	if err != nil {
		t.Fatalf("expected parse success, got: %v", err)
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
