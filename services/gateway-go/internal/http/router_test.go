package http

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestHealthzRoute(t *testing.T) {
	app := NewRouter()
	resp := ut.PerformRequest(app.Engine, "GET", "/healthz", nil)
	if resp.Code != 200 {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("expected status=ok, got %#v", body["status"])
	}
	if body["service"] != "gateway-go" {
		t.Fatalf("expected service=gateway-go, got %#v", body["service"])
	}
}

func TestIngestRiskEventRoute(t *testing.T) {
	app := NewRouter()
	payload := `{
		"merchant_id":"merchant-1001",
		"event_type":"abnormal_listing_activity",
		"evidence":["sku_spike","ip_anomaly"],
		"risk_score":0.87
	}`

	resp := ut.PerformRequest(
		app.Engine,
		"POST",
		"/api/v1/risk/events/ingest",
		&ut.Body{Body: strings.NewReader(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	if resp.Code != 202 {
		t.Fatalf("expected status 202, got %d", resp.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if body["accepted"] != true {
		t.Fatalf("expected accepted=true, got %#v", body["accepted"])
	}
	if body["case_id"] == "" {
		t.Fatalf("expected non-empty case_id")
	}
}

func TestGetRiskCaseRoute(t *testing.T) {
	app := NewRouter()
	resp := ut.PerformRequest(app.Engine, "GET", "/api/v1/risk/cases/case-risk-001", nil)
	if resp.Code != 200 {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if body["case_id"] != "case-risk-001" {
		t.Fatalf("expected case_id=case-risk-001, got %#v", body["case_id"])
	}
	if body["merchant_id"] == "" {
		t.Fatalf("expected non-empty merchant_id")
	}
	evidence, ok := body["evidence_items"].([]any)
	if !ok || len(evidence) == 0 {
		t.Fatalf("expected non-empty evidence_items, got %#v", body["evidence_items"])
	}
}
