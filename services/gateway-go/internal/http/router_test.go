package http

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/ut"

	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/mq"
	"trustops-ecommerce-riskops-platform/services/gateway-go/internal/storage"
)

type fakeRepository struct {
	cases          map[string]storage.Case
	creates        int
	ingests        int
	lastIngest     storage.IngestInput
	lastReplay     bool
	replayByKey    map[string]storage.IngestResult
	metrics        storage.OpsMetrics
	auditLogs      map[string][]storage.AuditLog
	pendingOutbox  []storage.OutboxItem
	outboxFailures map[int64]int
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

func (f *fakeRepository) IngestCase(_ context.Context, input storage.IngestInput) (storage.IngestResult, error) {
	f.ingests++
	f.lastIngest = input
	if f.replayByKey != nil {
		if replay, ok := f.replayByKey[input.IdempotencyKey]; ok {
			f.lastReplay = true
			return replay, nil
		}
	}
	return storage.IngestResult{
		Case:             input.Case,
		IdempotentReplay: false,
		ReplayCount:      0,
	}, nil
}

func (f *fakeRepository) GetOpsMetrics(_ context.Context) (storage.OpsMetrics, error) {
	return f.metrics, nil
}

func (f *fakeRepository) ListAuditLogs(_ context.Context, caseID string, limit int) ([]storage.AuditLog, error) {
	items := append([]storage.AuditLog(nil), f.auditLogs[caseID]...)
	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

func (f *fakeRepository) ListPendingOutbox(_ context.Context, _ time.Time, _ int) ([]storage.OutboxItem, error) {
	return f.pendingOutbox, nil
}

func (f *fakeRepository) MarkOutboxPublished(_ context.Context, _ int64) error {
	return nil
}

func (f *fakeRepository) MarkOutboxFailed(_ context.Context, item storage.OutboxItem, _ string, _ time.Time, _ int) (bool, error) {
	if f.outboxFailures == nil {
		f.outboxFailures = map[int64]int{}
	}
	f.outboxFailures[item.ID]++
	return false, nil
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
	app := NewRouter(Dependencies{
		Repository:    repo,
		Authenticator: NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:   NewInMemoryRateLimiter(10),
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
		ut.Header{Key: "X-API-Key", Value: "ops-key"},
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
	if repo.ingests != 1 {
		t.Fatalf("expected ingest to be called once, got %d", repo.ingests)
	}
	if body["idempotent_replay"] != false {
		t.Fatalf("expected idempotent_replay=false, got %#v", body["idempotent_replay"])
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
	app := NewRouter(Dependencies{
		Repository:    repo,
		Authenticator: NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:   NewInMemoryRateLimiter(10),
	})
	resp := ut.PerformRequest(app.Engine, "GET", "/api/v1/risk/cases/case-risk-001", nil, ut.Header{Key: "X-API-Key", Value: "ops-key"})
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
		Repository:    &fakeRepository{cases: map[string]storage.Case{}},
		Authenticator: NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:   NewInMemoryRateLimiter(10),
	})
	resp := ut.PerformRequest(app.Engine, "GET", "/api/v1/risk/cases/case-risk-404", nil, ut.Header{Key: "X-API-Key", Value: "ops-key"})
	if resp.Code != 404 {
		t.Fatalf("expected status 404, got %d", resp.Code)
	}
}

func TestIngestRiskEventRouteIdempotencyHeader(t *testing.T) {
	repo := &fakeRepository{}
	app := NewRouter(Dependencies{
		Repository:    repo,
		Authenticator: NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:   NewInMemoryRateLimiter(10),
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
		ut.Header{Key: "X-API-Key", Value: "ops-key"},
		ut.Header{Key: "X-Idempotency-Key", Value: "idem-manual-key"},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	if resp.Code != 202 {
		t.Fatalf("expected status 202, got %d", resp.Code)
	}
	if repo.lastIngest.IdempotencyKey != "idem-manual-key" {
		t.Fatalf("expected explicit idempotency key to be used, got %q", repo.lastIngest.IdempotencyKey)
	}
}

func TestIngestRiskEventRouteIdempotentReplay(t *testing.T) {
	repo := &fakeRepository{
		replayByKey: map[string]storage.IngestResult{
			"idem-replay": {
				Case: storage.Case{
					CaseID:       "case-existing-001",
					MerchantID:   "merchant-1001",
					RiskScore:    0.91,
					CaseStatus:   "pending_review",
					EventType:    "abnormal_listing_activity",
					OccurredAtMs: 1_710_000_000_000,
				},
				IdempotentReplay: true,
				ReplayCount:      2,
			},
		},
	}
	app := NewRouter(Dependencies{
		Repository:    repo,
		Authenticator: NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:   NewInMemoryRateLimiter(10),
		IDGenerator: func() string {
			return "case-risk-new-ignored"
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
		ut.Header{Key: "X-API-Key", Value: "ops-key"},
		ut.Header{Key: "X-Idempotency-Key", Value: "idem-replay"},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	if resp.Code != 202 {
		t.Fatalf("expected status 202, got %d", resp.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	if body["idempotent_replay"] != true {
		t.Fatalf("expected idempotent_replay=true, got %#v", body["idempotent_replay"])
	}
	if body["case_id"] != "case-existing-001" {
		t.Fatalf("expected replayed case_id, got %#v", body["case_id"])
	}
}

func TestGetOpsMetricsRoute(t *testing.T) {
	app := NewRouter(Dependencies{
		Repository: &fakeRepository{
			metrics: storage.OpsMetrics{
				TotalIngests:         12,
				IdempotentReplays:    3,
				PendingOutboxEvents:  5,
				DeadLetterOutboxRows: 1,
				AuditLogCount:        8,
			},
		},
		Authenticator: NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:   NewInMemoryRateLimiter(10),
	})

	resp := ut.PerformRequest(app.Engine, "GET", "/api/v1/risk/ops/metrics", nil, ut.Header{Key: "X-API-Key", Value: "ops-key"})
	if resp.Code != 200 {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
}

func TestIngestRiskEventRouteRejectsMissingAPIKey(t *testing.T) {
	app := NewRouter(Dependencies{
		Repository:    &fakeRepository{},
		Authenticator: NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:   NewInMemoryRateLimiter(10),
	})
	payload := `{"merchant_id":"merchant-1001","event_type":"abnormal_listing_activity","evidence":["sku_spike"],"risk_score":0.87}`

	resp := ut.PerformRequest(
		app.Engine,
		"POST",
		"/api/v1/risk/events/ingest",
		&ut.Body{Body: strings.NewReader(payload), Len: len(payload)},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	if resp.Code != 401 {
		t.Fatalf("expected status 401, got %d", resp.Code)
	}
}

func TestIngestRiskEventRouteRejectsInvalidAPIKey(t *testing.T) {
	app := NewRouter(Dependencies{
		Repository:    &fakeRepository{},
		Authenticator: NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:   NewInMemoryRateLimiter(10),
	})
	payload := `{"merchant_id":"merchant-1001","event_type":"abnormal_listing_activity","evidence":["sku_spike"],"risk_score":0.87}`

	resp := ut.PerformRequest(
		app.Engine,
		"POST",
		"/api/v1/risk/events/ingest",
		&ut.Body{Body: strings.NewReader(payload), Len: len(payload)},
		ut.Header{Key: "X-API-Key", Value: "bad-key"},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	if resp.Code != 403 {
		t.Fatalf("expected status 403, got %d", resp.Code)
	}
}

func TestIngestRiskEventRouteRateLimited(t *testing.T) {
	repo := &fakeRepository{}
	app := NewRouter(Dependencies{
		Repository:    repo,
		Authenticator: NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:   NewInMemoryRateLimiter(1),
		IDGenerator: func() string {
			return "case-risk-001"
		},
		Clock: func() time.Time {
			return time.Unix(1_710_000_000, 0)
		},
	})
	payload := `{"merchant_id":"merchant-1001","event_type":"abnormal_listing_activity","evidence":["sku_spike"],"risk_score":0.87}`

	first := ut.PerformRequest(
		app.Engine,
		"POST",
		"/api/v1/risk/events/ingest",
		&ut.Body{Body: strings.NewReader(payload), Len: len(payload)},
		ut.Header{Key: "X-API-Key", Value: "ops-key"},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	if first.Code != 202 {
		t.Fatalf("expected first request status 202, got %d", first.Code)
	}

	second := ut.PerformRequest(
		app.Engine,
		"POST",
		"/api/v1/risk/events/ingest",
		&ut.Body{Body: strings.NewReader(payload), Len: len(payload)},
		ut.Header{Key: "X-API-Key", Value: "ops-key"},
		ut.Header{Key: "Content-Type", Value: "application/json"},
	)
	if second.Code != 429 {
		t.Fatalf("expected second request status 429, got %d", second.Code)
	}
}

func TestGetAuditLogsRoute(t *testing.T) {
	app := NewRouter(Dependencies{
		Repository: &fakeRepository{
			auditLogs: map[string][]storage.AuditLog{
				"case-risk-001": {
					{ID: 2, CaseID: "case-risk-001", Action: "idempotent_replay"},
					{ID: 1, CaseID: "case-risk-001", Action: "case_ingested"},
				},
			},
		},
		Authenticator: NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:   NewInMemoryRateLimiter(10),
	})

	resp := ut.PerformRequest(app.Engine, "GET", "/api/v1/risk/cases/case-risk-001/audit-logs?limit=1", nil, ut.Header{Key: "X-API-Key", Value: "ops-key"})
	if resp.Code != 200 {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse body: %v", err)
	}
	items, ok := body["items"].([]any)
	if !ok {
		t.Fatalf("expected items array, got %#v", body["items"])
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestMetricsRoutePrometheus(t *testing.T) {
	runtimeMetrics := NewRuntimeMetrics()
	runtimeMetrics.IncAuthRejects()
	runtimeMetrics.IncRateLimitRejects()
	app := NewRouter(Dependencies{
		Repository: &fakeRepository{
			metrics: storage.OpsMetrics{
				TotalIngests:         12,
				IdempotentReplays:    3,
				PendingOutboxEvents:  5,
				DeadLetterOutboxRows: 1,
				AuditLogCount:        8,
			},
		},
		Authenticator:  NewAPIKeyAuthenticator([]string{"ops-key"}),
		RateLimiter:    NewInMemoryRateLimiter(10),
		RuntimeMetrics: runtimeMetrics,
	})

	resp := ut.PerformRequest(app.Engine, "GET", "/metrics", nil, ut.Header{Key: "X-API-Key", Value: "ops-key"})
	if resp.Code != 200 {
		t.Fatalf("expected status 200, got %d", resp.Code)
	}
	body := string(resp.Body.Bytes())
	for _, needle := range []string{
		"trustops_riskops_total_ingests 12",
		"trustops_riskops_idempotent_replays_total 3",
		"trustops_riskops_pending_outbox_events 5",
		"trustops_riskops_dead_letter_outbox_rows 1",
		"trustops_riskops_audit_log_count 8",
		"trustops_riskops_auth_rejects_total 1",
		"trustops_riskops_rate_limit_rejects_total 1",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("expected metrics body to contain %q, got %q", needle, body)
		}
	}
}
