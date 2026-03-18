package http

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/mq"
	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/storage"
)

type fakeRepository struct {
	cases   map[string]storage.Case
	creates int
}

func (f *fakeRepository) CreateCase(_ context.Context, c storage.Case) error {
	if f.cases == nil {
		f.cases = make(map[string]storage.Case)
	}
	f.creates++
	f.cases[c.CaseID] = c
	return nil
}

func (f *fakeRepository) GetCase(_ context.Context, caseID string) (storage.Case, error) {
	c, ok := f.cases[caseID]
	if !ok {
		return storage.Case{}, storage.ErrCaseNotFound
	}
	return c, nil
}

type fakePublisher struct {
	events []mq.CaseIngestedEvent
	err    error
}

func (f *fakePublisher) PublishCaseIngested(_ context.Context, event mq.CaseIngestedEvent) error {
	if f.err != nil {
		return f.err
	}
	f.events = append(f.events, event)
	return nil
}

func TestHealthzRoute(t *testing.T) {
	app := NewRouter(Dependencies{})
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
	repo := &fakeRepository{}
	publisher := &fakePublisher{}
	app := NewRouter(Dependencies{
		Repository: repo,
		Publisher:  publisher,
		IDGenerator: func() string {
			return "case-risk-001"
		},
		Clock: func() time.Time {
			return time.UnixMilli(1_710_000_000_000)
		},
	})
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
	if body["case_id"] != "case-risk-001" {
		t.Fatalf("expected case_id=case-risk-001, got %#v", body["case_id"])
	}
	if repo.creates != 1 {
		t.Fatalf("expected create to be called once, got %d", repo.creates)
	}
	if len(publisher.events) != 1 {
		t.Fatalf("expected one event to be published, got %d", len(publisher.events))
	}
	if publisher.events[0].CaseID != "case-risk-001" {
		t.Fatalf("unexpected published event: %#v", publisher.events[0])
	}
}

func TestGetRiskCaseRoute(t *testing.T) {
	repo := &fakeRepository{
		cases: map[string]storage.Case{
			"case-risk-001": {
				CaseID:       "case-risk-001",
				MerchantID:   "merchant-1001",
				RiskCategory: "listing_fraud",
				CaseStatus:   "pending_review",
				EvidenceItems: []string{
					"sku_spike",
					"ip_anomaly",
				},
			},
		},
	}
	app := NewRouter(Dependencies{Repository: repo})
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

func TestGetRiskCaseRouteNotFound(t *testing.T) {
	app := NewRouter(Dependencies{
		Repository: &fakeRepository{cases: map[string]storage.Case{}},
	})
	resp := ut.PerformRequest(app.Engine, "GET", "/api/v1/risk/cases/case-risk-404", nil)
	if resp.Code != 404 {
		t.Fatalf("expected status 404, got %d", resp.Code)
	}
}

func TestIngestRiskEventRoutePublisherError(t *testing.T) {
	app := NewRouter(Dependencies{
		Repository: &fakeRepository{},
		Publisher: &fakePublisher{
			err: errors.New("publish failed"),
		},
	})
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
}
