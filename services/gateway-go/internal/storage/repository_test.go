package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

type fakeCaseCache struct {
	items map[string]Case
}

func (f *fakeCaseCache) GetCase(_ context.Context, caseID string) (Case, bool, error) {
	c, ok := f.items[caseID]
	return c, ok, nil
}

func (f *fakeCaseCache) SetCase(_ context.Context, c Case, _ time.Duration) error {
	if f.items == nil {
		f.items = make(map[string]Case)
	}
	f.items[c.CaseID] = c
	return nil
}

type fakeBaseRepo struct {
	getCalls int
	cases    map[string]Case
}

func (f *fakeBaseRepo) CreateCase(_ context.Context, c Case) error {
	if f.cases == nil {
		f.cases = make(map[string]Case)
	}
	f.cases[c.CaseID] = c
	return nil
}

func (f *fakeBaseRepo) GetCase(_ context.Context, caseID string) (Case, error) {
	f.getCalls++
	c, ok := f.cases[caseID]
	if !ok {
		return Case{}, ErrCaseNotFound
	}
	return c, nil
}

func (f *fakeBaseRepo) IngestCase(_ context.Context, input IngestInput) (IngestResult, error) {
	if err := f.CreateCase(context.Background(), input.Case); err != nil {
		return IngestResult{}, err
	}
	return IngestResult{Case: input.Case}, nil
}

func (f *fakeBaseRepo) GetOpsMetrics(_ context.Context) (OpsMetrics, error) {
	return OpsMetrics{}, nil
}

func (f *fakeBaseRepo) ListAuditLogs(_ context.Context, _ string, _ int) ([]AuditLog, error) {
	return nil, nil
}

func (f *fakeBaseRepo) ListPendingOutbox(_ context.Context, _ time.Time, _ int) ([]OutboxItem, error) {
	return nil, nil
}

func (f *fakeBaseRepo) MarkOutboxPublished(_ context.Context, _ int64) error {
	return nil
}

func (f *fakeBaseRepo) MarkOutboxFailed(_ context.Context, _ OutboxItem, _ string, _ time.Time, _ int) (bool, error) {
	return false, nil
}

func TestInMemoryRepositoryCreateAndGetCase(t *testing.T) {
	repo := NewInMemoryRepository()
	input := Case{
		CaseID:       "case-risk-001",
		MerchantID:   "merchant-1001",
		EventType:    "abnormal_listing_activity",
		RiskCategory: "listing_fraud",
		CaseStatus:   "pending_review",
		EvidenceItems: []string{
			"sku_spike",
			"ip_anomaly",
		},
		RiskScore:    0.87,
		OccurredAtMs: 1_710_000_000_000,
		TriggeredBy:  "rule-engine",
	}

	if err := repo.CreateCase(context.Background(), input); err != nil {
		t.Fatalf("create case failed: %v", err)
	}

	got, err := repo.GetCase(context.Background(), "case-risk-001")
	if err != nil {
		t.Fatalf("get case failed: %v", err)
	}
	if got.CaseID != input.CaseID || got.MerchantID != input.MerchantID {
		t.Fatalf("unexpected case loaded: %#v", got)
	}
	if len(got.EvidenceItems) != 2 {
		t.Fatalf("unexpected evidence count: %d", len(got.EvidenceItems))
	}
}

func TestCachedRepositoryUsesCacheOnSecondRead(t *testing.T) {
	base := &fakeBaseRepo{
		cases: map[string]Case{
			"case-risk-001": {
				CaseID:       "case-risk-001",
				MerchantID:   "merchant-1001",
				RiskCategory: "listing_fraud",
				CaseStatus:   "pending_review",
				EvidenceItems: []string{
					"sku_spike",
				},
			},
		},
	}
	cache := &fakeCaseCache{items: make(map[string]Case)}
	repo := NewCachedRepository(base, cache, 3*time.Minute)

	_, err := repo.GetCase(context.Background(), "case-risk-001")
	if err != nil {
		t.Fatalf("first get should succeed, got: %v", err)
	}
	_, err = repo.GetCase(context.Background(), "case-risk-001")
	if err != nil {
		t.Fatalf("second get should succeed, got: %v", err)
	}

	if base.getCalls != 1 {
		t.Fatalf("expected base repo to be read once, got %d", base.getCalls)
	}
}

func TestMySQLRepositoryCreateAndGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewMySQLRepository(db)
	input := Case{
		CaseID:       "case-risk-001",
		MerchantID:   "merchant-1001",
		EventType:    "abnormal_listing_activity",
		RiskCategory: "listing_fraud",
		CaseStatus:   "pending_review",
		EvidenceItems: []string{
			"sku_spike",
			"ip_anomaly",
		},
		RiskScore:    0.87,
		OccurredAtMs: 1_710_000_000_000,
		TriggeredBy:  "rule-engine",
	}

	mock.ExpectExec("INSERT INTO risk_cases").
		WithArgs(
			input.CaseID,
			input.MerchantID,
			input.EventType,
			input.RiskCategory,
			input.CaseStatus,
			input.RiskScore,
			`["sku_spike","ip_anomaly"]`,
			input.TriggeredBy,
			input.OccurredAtMs,
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.CreateCase(context.Background(), input); err != nil {
		t.Fatalf("create case failed: %v", err)
	}

	rows := sqlmock.NewRows([]string{
		"case_id", "merchant_id", "event_type", "risk_category", "case_status", "risk_score", "evidence_json", "triggered_by", "occurred_at_ms", "created_at_unix",
	}).AddRow(
		input.CaseID, input.MerchantID, input.EventType, input.RiskCategory, input.CaseStatus, input.RiskScore, `["sku_spike","ip_anomaly"]`, input.TriggeredBy, input.OccurredAtMs, int64(1_710_000_001),
	)

	mock.ExpectQuery("SELECT case_id, merchant_id, event_type, risk_category, case_status, risk_score, evidence_json, triggered_by, occurred_at_ms, UNIX_TIMESTAMP\\(created_at\\) AS created_at_unix FROM risk_cases WHERE case_id = \\?").
		WithArgs(input.CaseID).
		WillReturnRows(rows)

	got, err := repo.GetCase(context.Background(), input.CaseID)
	if err != nil {
		t.Fatalf("get case failed: %v", err)
	}
	if got.CaseID != input.CaseID {
		t.Fatalf("unexpected case_id: %q", got.CaseID)
	}
	if len(got.EvidenceItems) != 2 {
		t.Fatalf("unexpected evidence size: %d", len(got.EvidenceItems))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestInMemoryRepositoryCaseNotFound(t *testing.T) {
	repo := NewInMemoryRepository()

	_, err := repo.GetCase(context.Background(), "unknown")
	if !errors.Is(err, ErrCaseNotFound) {
		t.Fatalf("expected ErrCaseNotFound, got %v", err)
	}
}

func TestInMemoryRepositoryIngestCaseDeduplicatesByIdempotencyKey(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	input := IngestInput{
		IdempotencyKey: "idem-risk-001",
		Case: Case{
			CaseID:        "case-risk-001",
			MerchantID:    "merchant-1001",
			EventType:     "abnormal_listing_activity",
			RiskCategory:  "listing_fraud",
			CaseStatus:    "pending_review",
			EvidenceItems: []string{"sku_spike", "ip_anomaly"},
			RiskScore:     0.87,
			OccurredAtMs:  1_710_000_000_000,
		},
	}

	first, err := repo.IngestCase(ctx, input)
	if err != nil {
		t.Fatalf("first IngestCase() error = %v", err)
	}
	second, err := repo.IngestCase(ctx, input)
	if err != nil {
		t.Fatalf("second IngestCase() error = %v", err)
	}

	if first.IdempotentReplay {
		t.Fatalf("expected first ingest not to be replay")
	}
	if !second.IdempotentReplay {
		t.Fatalf("expected second ingest to be replay")
	}
	if second.Case.CaseID != first.Case.CaseID {
		t.Fatalf("expected replay to reuse case ID")
	}

	metrics, err := repo.GetOpsMetrics(ctx)
	if err != nil {
		t.Fatalf("GetOpsMetrics() error = %v", err)
	}
	if metrics.TotalIngests != 1 {
		t.Fatalf("expected 1 unique ingest, got %d", metrics.TotalIngests)
	}
	if metrics.IdempotentReplays != 1 {
		t.Fatalf("expected 1 replay, got %d", metrics.IdempotentReplays)
	}
	if metrics.PendingOutboxEvents != 1 {
		t.Fatalf("expected 1 pending outbox row, got %d", metrics.PendingOutboxEvents)
	}
	if metrics.AuditLogCount != 2 {
		t.Fatalf("expected 2 audit logs, got %d", metrics.AuditLogCount)
	}
}

func TestInMemoryRepositoryMarksOutboxDeadLetterAfterFailures(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	_, err := repo.IngestCase(ctx, IngestInput{
		IdempotencyKey: "idem-risk-dead",
		Case: Case{
			CaseID:        "case-risk-dead",
			MerchantID:    "merchant-1001",
			EventType:     "listing_fraud",
			RiskCategory:  "listing_fraud",
			CaseStatus:    "pending_review",
			EvidenceItems: []string{"velocity_anomaly"},
			RiskScore:     0.93,
			OccurredAtMs:  1_710_000_100_000,
		},
	})
	if err != nil {
		t.Fatalf("IngestCase() error = %v", err)
	}

	items, err := repo.ListPendingOutbox(ctx, time.Now().UTC(), 10)
	if err != nil {
		t.Fatalf("ListPendingOutbox() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 pending outbox item, got %d", len(items))
	}

	dead, err := repo.MarkOutboxFailed(ctx, items[0], "publish failed", time.Now().UTC().Add(time.Minute), 1)
	if err != nil {
		t.Fatalf("MarkOutboxFailed() error = %v", err)
	}
	if !dead {
		t.Fatalf("expected outbox item to be dead-lettered")
	}

	metrics, err := repo.GetOpsMetrics(ctx)
	if err != nil {
		t.Fatalf("GetOpsMetrics() error = %v", err)
	}
	if metrics.DeadLetterOutboxRows != 1 {
		t.Fatalf("expected 1 dead-letter row, got %d", metrics.DeadLetterOutboxRows)
	}
	if metrics.AuditLogCount != 2 {
		t.Fatalf("expected 2 audit logs, got %d", metrics.AuditLogCount)
	}
}

func TestInMemoryRepositoryListAuditLogsNewestFirst(t *testing.T) {
	repo := NewInMemoryRepository()
	ctx := context.Background()
	input := IngestInput{
		IdempotencyKey: "idem-risk-001",
		Case: Case{
			CaseID:        "case-risk-001",
			MerchantID:    "merchant-1001",
			EventType:     "abnormal_listing_activity",
			RiskCategory:  "listing_fraud",
			CaseStatus:    "pending_review",
			EvidenceItems: []string{"sku_spike", "ip_anomaly"},
			RiskScore:     0.87,
			OccurredAtMs:  1_710_000_000_000,
		},
	}

	if _, err := repo.IngestCase(ctx, input); err != nil {
		t.Fatalf("IngestCase() error = %v", err)
	}
	if _, err := repo.IngestCase(ctx, input); err != nil {
		t.Fatalf("second IngestCase() error = %v", err)
	}

	logs, err := repo.ListAuditLogs(ctx, "case-risk-001", 10)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 audit logs, got %d", len(logs))
	}
	if logs[0].Action != "idempotent_replay" {
		t.Fatalf("expected newest log to be idempotent_replay, got %q", logs[0].Action)
	}
	if logs[1].Action != "case_ingested" {
		t.Fatalf("expected older log to be case_ingested, got %q", logs[1].Action)
	}
}

func TestMySQLRepositoryListAuditLogs(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	repo := NewMySQLRepository(db)
	rows := sqlmock.NewRows([]string{"id", "case_id", "action", "detail_json", "created_at_unix"}).
		AddRow(2, "case-risk-001", "idempotent_replay", `{"replay_count":1}`, int64(1_710_000_001)).
		AddRow(1, "case-risk-001", "case_ingested", `{"event_type":"abnormal_listing_activity"}`, int64(1_710_000_000))

	mock.ExpectQuery("SELECT id, case_id, action, detail_json, UNIX_TIMESTAMP\\(created_at\\) AS created_at_unix FROM risk_audit_logs WHERE case_id = \\? ORDER BY id DESC LIMIT \\?").
		WithArgs("case-risk-001", 2).
		WillReturnRows(rows)

	logs, err := repo.ListAuditLogs(context.Background(), "case-risk-001", 2)
	if err != nil {
		t.Fatalf("ListAuditLogs() error = %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 audit logs, got %d", len(logs))
	}
	if logs[0].Action != "idempotent_replay" {
		t.Fatalf("unexpected first action %q", logs[0].Action)
	}
	if got := logs[1].Detail["event_type"]; got != "abnormal_listing_activity" {
		t.Fatalf("unexpected detail event_type %#v", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
