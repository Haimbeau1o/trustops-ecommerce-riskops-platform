package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var (
	ErrCaseNotFound         = errors.New("case_not_found")
	ErrIdempotencyKeyMissed = errors.New("idempotency_key_required")
)

// Case represents the risk case entity persisted by the gateway.
type Case struct {
	CaseID        string    `json:"case_id"`
	MerchantID    string    `json:"merchant_id"`
	EventType     string    `json:"event_type"`
	RiskCategory  string    `json:"risk_category"`
	CaseStatus    string    `json:"case_status"`
	EvidenceItems []string  `json:"evidence_items"`
	RiskScore     float64   `json:"risk_score"`
	TriggeredBy   string    `json:"triggered_by,omitempty"`
	OccurredAtMs  int64     `json:"occurred_at_ms"`
	CreatedAt     time.Time `json:"created_at"`
}

// IngestInput contains the idempotent ingest command handled by the gateway.
type IngestInput struct {
	IdempotencyKey string
	Case           Case
}

// IngestResult reports whether the ingest created new state or replayed an existing case.
type IngestResult struct {
	Case             Case
	IdempotentReplay bool
	ReplayCount      int
}

// AuditLog captures one auditable state transition or reliability event.
type AuditLog struct {
	ID        int64          `json:"id"`
	CaseID    string         `json:"case_id"`
	Action    string         `json:"action"`
	Detail    map[string]any `json:"detail"`
	CreatedAt time.Time      `json:"created_at"`
}

// OpsMetrics summarizes JD-aligned runtime reliability signals.
type OpsMetrics struct {
	TotalIngests         int64 `json:"total_ingests"`
	IdempotentReplays    int64 `json:"idempotent_replays"`
	PendingOutboxEvents  int64 `json:"pending_outbox_events"`
	DeadLetterOutboxRows int64 `json:"dead_letter_outbox_rows"`
	AuditLogCount        int64 `json:"audit_log_count"`
}

// OutboxItem represents one pending queue dispatch job.
type OutboxItem struct {
	ID              int64
	CaseID          string
	EventType       string
	Payload         []byte
	PublishAttempts int
}

// Repository defines the storage abstraction for risk cases and async handoff state.
type Repository interface {
	CreateCase(ctx context.Context, c Case) error
	IngestCase(ctx context.Context, input IngestInput) (IngestResult, error)
	GetCase(ctx context.Context, caseID string) (Case, error)
	GetOpsMetrics(ctx context.Context) (OpsMetrics, error)
	ListAuditLogs(ctx context.Context, caseID string, limit int) ([]AuditLog, error)
	ListPendingOutbox(ctx context.Context, now time.Time, limit int) ([]OutboxItem, error)
	MarkOutboxPublished(ctx context.Context, id int64) error
	MarkOutboxFailed(ctx context.Context, item OutboxItem, lastError string, nextAttemptAt time.Time, maxAttempts int) (bool, error)
}

// CaseCache defines cache operations for case lookup.
type CaseCache interface {
	GetCase(ctx context.Context, caseID string) (Case, bool, error)
	SetCase(ctx context.Context, c Case, ttl time.Duration) error
}

type ingestRecord struct {
	CaseID      string
	ReplayCount int
}

type inMemoryOutboxItem struct {
	OutboxItem
	Status        string
	NextAttemptAt time.Time
}

// InMemoryRepository is an in-process fallback repository.
type InMemoryRepository struct {
	mu            sync.RWMutex
	cases         map[string]Case
	ingests       map[string]ingestRecord
	outbox        map[int64]inMemoryOutboxItem
	nextOutboxID  int64
	auditLogs     []AuditLog
	nextAuditID   int64
	auditLogCount int64
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		cases:   make(map[string]Case),
		ingests: make(map[string]ingestRecord),
		outbox:  make(map[int64]inMemoryOutboxItem),
	}
}

func (r *InMemoryRepository) CreateCase(_ context.Context, c Case) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storeCase(c)
	return nil
}

func (r *InMemoryRepository) IngestCase(_ context.Context, input IngestInput) (IngestResult, error) {
	if input.IdempotencyKey == "" {
		return IngestResult{}, ErrIdempotencyKeyMissed
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if record, ok := r.ingests[input.IdempotencyKey]; ok {
		record.ReplayCount++
		r.ingests[input.IdempotencyKey] = record
		existing := r.cases[record.CaseID]
		r.appendAuditLocked(record.CaseID, "idempotent_replay", map[string]any{
			"idempotency_key": input.IdempotencyKey,
			"replay_count":    record.ReplayCount,
		})
		return IngestResult{
			Case:             existing,
			IdempotentReplay: true,
			ReplayCount:      record.ReplayCount,
		}, nil
	}

	r.storeCase(input.Case)
	r.ingests[input.IdempotencyKey] = ingestRecord{CaseID: input.Case.CaseID}
	r.nextOutboxID++
	payload, err := buildOutboxPayload(input)
	if err != nil {
		return IngestResult{}, err
	}
	r.outbox[r.nextOutboxID] = inMemoryOutboxItem{
		OutboxItem: OutboxItem{
			ID:              r.nextOutboxID,
			CaseID:          input.Case.CaseID,
			EventType:       "risk_case_ingested",
			Payload:         payload,
			PublishAttempts: 0,
		},
		Status:        "pending",
		NextAttemptAt: time.Now().UTC(),
	}
	r.appendAuditLocked(input.Case.CaseID, "case_ingested", map[string]any{
		"idempotency_key": input.IdempotencyKey,
		"event_type":      input.Case.EventType,
	})
	return IngestResult{Case: input.Case}, nil
}

func (r *InMemoryRepository) GetCase(_ context.Context, caseID string) (Case, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.cases[caseID]
	if !ok {
		return Case{}, ErrCaseNotFound
	}
	return c, nil
}

func (r *InMemoryRepository) GetOpsMetrics(_ context.Context) (OpsMetrics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metrics := OpsMetrics{
		TotalIngests:      int64(len(r.ingests)),
		IdempotentReplays: 0,
	}
	for _, record := range r.ingests {
		metrics.IdempotentReplays += int64(record.ReplayCount)
	}
	for _, item := range r.outbox {
		switch item.Status {
		case "pending", "retry", "publishing":
			metrics.PendingOutboxEvents++
		case "dead_letter":
			metrics.DeadLetterOutboxRows++
		}
	}
	metrics.AuditLogCount = r.auditLogCount
	return metrics, nil
}

func (r *InMemoryRepository) ListAuditLogs(_ context.Context, caseID string, limit int) ([]AuditLog, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 20
	}
	logs := make([]AuditLog, 0, limit)
	for i := len(r.auditLogs) - 1; i >= 0; i-- {
		entry := r.auditLogs[i]
		if entry.CaseID != caseID {
			continue
		}
		logs = append(logs, cloneAuditLog(entry))
		if len(logs) >= limit {
			break
		}
	}
	return logs, nil
}

func (r *InMemoryRepository) ListPendingOutbox(_ context.Context, now time.Time, limit int) ([]OutboxItem, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := make([]OutboxItem, 0, limit)
	for id, item := range r.outbox {
		if len(items) >= limit {
			break
		}
		if (item.Status == "pending" || item.Status == "retry") && !item.NextAttemptAt.After(now) {
			item.Status = "publishing"
			r.outbox[id] = item
			items = append(items, item.OutboxItem)
		}
	}
	return items, nil
}

func (r *InMemoryRepository) MarkOutboxPublished(_ context.Context, id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	item, ok := r.outbox[id]
	if !ok {
		return nil
	}
	item.Status = "published"
	item.PublishAttempts++
	r.outbox[id] = item
	return nil
}

func (r *InMemoryRepository) MarkOutboxFailed(_ context.Context, item OutboxItem, lastError string, nextAttemptAt time.Time, maxAttempts int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.outbox[item.ID]
	if !ok {
		return false, nil
	}
	current.PublishAttempts++
	if current.PublishAttempts >= maxAttempts {
		current.Status = "dead_letter"
		r.outbox[item.ID] = current
		r.appendAuditLocked(current.CaseID, "outbox_dead_letter", map[string]any{
			"outbox_id": item.ID,
			"error":     lastError,
		})
		return true, nil
	}
	current.Status = "retry"
	current.NextAttemptAt = nextAttemptAt
	r.outbox[item.ID] = current
	r.appendAuditLocked(current.CaseID, "outbox_retry_scheduled", map[string]any{
		"outbox_id":        item.ID,
		"error":            lastError,
		"next_attempt_at":  nextAttemptAt.UTC().Format(time.RFC3339),
		"publish_attempts": current.PublishAttempts,
	})
	return false, nil
}

func (r *InMemoryRepository) storeCase(c Case) {
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	r.cases[c.CaseID] = c
}

func (r *InMemoryRepository) appendAuditLocked(caseID, action string, detail map[string]any) {
	r.nextAuditID++
	r.auditLogCount++
	r.auditLogs = append(r.auditLogs, AuditLog{
		ID:        r.nextAuditID,
		CaseID:    caseID,
		Action:    action,
		Detail:    cloneDetail(detail),
		CreatedAt: time.Now().UTC(),
	})
}

// CachedRepository is a decorator that adds read-through/write-through caching.
type CachedRepository struct {
	base  Repository
	cache CaseCache
	ttl   time.Duration
}

func NewCachedRepository(base Repository, cache CaseCache, ttl time.Duration) *CachedRepository {
	return &CachedRepository{
		base:  base,
		cache: cache,
		ttl:   ttl,
	}
}

func (r *CachedRepository) CreateCase(ctx context.Context, c Case) error {
	if err := r.base.CreateCase(ctx, c); err != nil {
		return err
	}
	_ = r.cache.SetCase(ctx, c, r.ttl)
	return nil
}

func (r *CachedRepository) IngestCase(ctx context.Context, input IngestInput) (IngestResult, error) {
	result, err := r.base.IngestCase(ctx, input)
	if err != nil {
		return IngestResult{}, err
	}
	_ = r.cache.SetCase(ctx, result.Case, r.ttl)
	return result, nil
}

func (r *CachedRepository) GetCase(ctx context.Context, caseID string) (Case, error) {
	cached, ok, err := r.cache.GetCase(ctx, caseID)
	if err == nil && ok {
		return cached, nil
	}

	c, err := r.base.GetCase(ctx, caseID)
	if err != nil {
		return Case{}, err
	}
	_ = r.cache.SetCase(ctx, c, r.ttl)
	return c, nil
}

func (r *CachedRepository) GetOpsMetrics(ctx context.Context) (OpsMetrics, error) {
	return r.base.GetOpsMetrics(ctx)
}

func (r *CachedRepository) ListAuditLogs(ctx context.Context, caseID string, limit int) ([]AuditLog, error) {
	return r.base.ListAuditLogs(ctx, caseID, limit)
}

func (r *CachedRepository) ListPendingOutbox(ctx context.Context, now time.Time, limit int) ([]OutboxItem, error) {
	return r.base.ListPendingOutbox(ctx, now, limit)
}

func (r *CachedRepository) MarkOutboxPublished(ctx context.Context, id int64) error {
	return r.base.MarkOutboxPublished(ctx, id)
}

func (r *CachedRepository) MarkOutboxFailed(ctx context.Context, item OutboxItem, lastError string, nextAttemptAt time.Time, maxAttempts int) (bool, error) {
	return r.base.MarkOutboxFailed(ctx, item, lastError, nextAttemptAt, maxAttempts)
}

// MySQLRepository persists cases and reliability metadata to MySQL via database/sql.
type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) CreateCase(ctx context.Context, c Case) error {
	return insertCase(ctx, r.db, c)
}

func (r *MySQLRepository) IngestCase(ctx context.Context, input IngestInput) (IngestResult, error) {
	if input.IdempotencyKey == "" {
		return IngestResult{}, ErrIdempotencyKeyMissed
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return IngestResult{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	var existingCaseID string
	var replayCount int
	err = tx.QueryRowContext(
		ctx,
		"SELECT case_id, replay_count FROM risk_ingest_idempotency WHERE idempotency_key = ? FOR UPDATE",
		input.IdempotencyKey,
	).Scan(&existingCaseID, &replayCount)
	switch {
	case err == nil:
		replayCount++
		if _, err := tx.ExecContext(
			ctx,
			"UPDATE risk_ingest_idempotency SET replay_count = ?, last_seen_at = CURRENT_TIMESTAMP WHERE idempotency_key = ?",
			replayCount,
			input.IdempotencyKey,
		); err != nil {
			return IngestResult{}, err
		}
		existingCase, err := getCase(ctx, tx, existingCaseID)
		if err != nil {
			return IngestResult{}, err
		}
		if err := insertAudit(ctx, tx, existingCaseID, "idempotent_replay", map[string]any{
			"idempotency_key": input.IdempotencyKey,
			"replay_count":    replayCount,
		}); err != nil {
			return IngestResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return IngestResult{}, err
		}
		return IngestResult{
			Case:             existingCase,
			IdempotentReplay: true,
			ReplayCount:      replayCount,
		}, nil
	case errors.Is(err, sql.ErrNoRows):
		if err := insertCase(ctx, tx, input.Case); err != nil {
			return IngestResult{}, err
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO risk_ingest_idempotency (idempotency_key, case_id, replay_count) VALUES (?, ?, 0)",
			input.IdempotencyKey,
			input.Case.CaseID,
		); err != nil {
			return IngestResult{}, err
		}
		payload, err := buildOutboxPayload(input)
		if err != nil {
			return IngestResult{}, err
		}
		if _, err := tx.ExecContext(
			ctx,
			"INSERT INTO risk_outbox_events (case_id, event_type, payload_json, status, publish_attempts, next_attempt_at) VALUES (?, ?, ?, 'pending', 0, CURRENT_TIMESTAMP)",
			input.Case.CaseID,
			"risk_case_ingested",
			string(payload),
		); err != nil {
			return IngestResult{}, err
		}
		if err := insertAudit(ctx, tx, input.Case.CaseID, "case_ingested", map[string]any{
			"idempotency_key": input.IdempotencyKey,
			"event_type":      input.Case.EventType,
		}); err != nil {
			return IngestResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return IngestResult{}, err
		}
		return IngestResult{Case: input.Case}, nil
	default:
		return IngestResult{}, err
	}
}

func (r *MySQLRepository) GetCase(ctx context.Context, caseID string) (Case, error) {
	return getCase(ctx, r.db, caseID)
}

func (r *MySQLRepository) GetOpsMetrics(ctx context.Context) (OpsMetrics, error) {
	totalIngests, err := countQuery(ctx, r.db, "SELECT COUNT(*) FROM risk_ingest_idempotency")
	if err != nil {
		return OpsMetrics{}, err
	}
	idempotentReplays, err := countQuery(ctx, r.db, "SELECT COALESCE(SUM(replay_count), 0) FROM risk_ingest_idempotency")
	if err != nil {
		return OpsMetrics{}, err
	}
	pendingOutbox, err := countQuery(ctx, r.db, "SELECT COUNT(*) FROM risk_outbox_events WHERE status IN ('pending', 'retry', 'publishing')")
	if err != nil {
		return OpsMetrics{}, err
	}
	deadLetterRows, err := countQuery(ctx, r.db, "SELECT COUNT(*) FROM risk_outbox_events WHERE status = 'dead_letter'")
	if err != nil {
		return OpsMetrics{}, err
	}
	auditLogCount, err := countQuery(ctx, r.db, "SELECT COUNT(*) FROM risk_audit_logs")
	if err != nil {
		return OpsMetrics{}, err
	}

	return OpsMetrics{
		TotalIngests:         totalIngests,
		IdempotentReplays:    idempotentReplays,
		PendingOutboxEvents:  pendingOutbox,
		DeadLetterOutboxRows: deadLetterRows,
		AuditLogCount:        auditLogCount,
	}, nil
}

func (r *MySQLRepository) ListAuditLogs(ctx context.Context, caseID string, limit int) ([]AuditLog, error) {
	if limit <= 0 {
		limit = 20
	}

	rows, err := r.db.QueryContext(
		ctx,
		"SELECT id, case_id, action, detail_json, UNIX_TIMESTAMP(created_at) AS created_at_unix FROM risk_audit_logs WHERE case_id = ? ORDER BY id DESC LIMIT ?",
		caseID,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]AuditLog, 0, limit)
	for rows.Next() {
		var entry AuditLog
		var detailJSON string
		var createdAtUnix int64
		if err := rows.Scan(&entry.ID, &entry.CaseID, &entry.Action, &detailJSON, &createdAtUnix); err != nil {
			return nil, err
		}
		if detailJSON != "" {
			if err := json.Unmarshal([]byte(detailJSON), &entry.Detail); err != nil {
				return nil, err
			}
		}
		entry.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
		logs = append(logs, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *MySQLRepository) ListPendingOutbox(ctx context.Context, now time.Time, limit int) ([]OutboxItem, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	rows, err := tx.QueryContext(
		ctx,
		`SELECT id, case_id, event_type, payload_json, publish_attempts
		FROM risk_outbox_events
		WHERE status IN ('pending', 'retry') AND next_attempt_at <= ?
		ORDER BY id
		LIMIT ?
		FOR UPDATE SKIP LOCKED`,
		now,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]OutboxItem, 0, limit)
	for rows.Next() {
		var item OutboxItem
		var payload string
		if err := rows.Scan(&item.ID, &item.CaseID, &item.EventType, &payload, &item.PublishAttempts); err != nil {
			return nil, err
		}
		item.Payload = []byte(payload)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, item := range items {
		if _, err := tx.ExecContext(
			ctx,
			"UPDATE risk_outbox_events SET status = 'publishing', locked_at = CURRENT_TIMESTAMP WHERE id = ?",
			item.ID,
		); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *MySQLRepository) MarkOutboxPublished(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(
		ctx,
		"UPDATE risk_outbox_events SET status = 'published', publish_attempts = publish_attempts + 1, last_error = '', locked_at = NULL, published_at = CURRENT_TIMESTAMP WHERE id = ?",
		id,
	)
	return err
}

func (r *MySQLRepository) MarkOutboxFailed(ctx context.Context, item OutboxItem, lastError string, nextAttemptAt time.Time, maxAttempts int) (bool, error) {
	attempts := item.PublishAttempts + 1
	if attempts >= maxAttempts {
		if _, err := r.db.ExecContext(
			ctx,
			"UPDATE risk_outbox_events SET status = 'dead_letter', publish_attempts = ?, last_error = ?, locked_at = NULL WHERE id = ?",
			attempts,
			lastError,
			item.ID,
		); err != nil {
			return false, err
		}
		if err := insertAudit(ctx, r.db, item.CaseID, "outbox_dead_letter", map[string]any{
			"outbox_id": item.ID,
			"error":     lastError,
		}); err != nil {
			return false, err
		}
		return true, nil
	}

	if _, err := r.db.ExecContext(
		ctx,
		"UPDATE risk_outbox_events SET status = 'retry', publish_attempts = ?, last_error = ?, next_attempt_at = ?, locked_at = NULL WHERE id = ?",
		attempts,
		lastError,
		nextAttemptAt,
		item.ID,
	); err != nil {
		return false, err
	}
	if err := insertAudit(ctx, r.db, item.CaseID, "outbox_retry_scheduled", map[string]any{
		"outbox_id":        item.ID,
		"error":            lastError,
		"next_attempt_at":  nextAttemptAt.UTC().Format(time.RFC3339),
		"publish_attempts": attempts,
	}); err != nil {
		return false, err
	}
	return false, nil
}

func insertCase(ctx context.Context, execer sqlExecer, c Case) error {
	evidenceJSON, err := json.Marshal(c.EvidenceItems)
	if err != nil {
		return err
	}
	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	const insertSQL = "INSERT INTO risk_cases (case_id, merchant_id, event_type, risk_category, case_status, risk_score, evidence_json, triggered_by, occurred_at_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"
	_, err = execer.ExecContext(
		ctx,
		insertSQL,
		c.CaseID,
		c.MerchantID,
		c.EventType,
		c.RiskCategory,
		c.CaseStatus,
		c.RiskScore,
		string(evidenceJSON),
		c.TriggeredBy,
		c.OccurredAtMs,
	)
	return err
}

func getCase(ctx context.Context, queryer sqlQueryer, caseID string) (Case, error) {
	const selectSQL = "SELECT case_id, merchant_id, event_type, risk_category, case_status, risk_score, evidence_json, triggered_by, occurred_at_ms, UNIX_TIMESTAMP(created_at) AS created_at_unix FROM risk_cases WHERE case_id = ?"

	var c Case
	var evidenceJSON string
	var createdAtUnix int64
	err := queryer.QueryRowContext(ctx, selectSQL, caseID).Scan(
		&c.CaseID,
		&c.MerchantID,
		&c.EventType,
		&c.RiskCategory,
		&c.CaseStatus,
		&c.RiskScore,
		&evidenceJSON,
		&c.TriggeredBy,
		&c.OccurredAtMs,
		&createdAtUnix,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Case{}, ErrCaseNotFound
		}
		return Case{}, err
	}
	if err := json.Unmarshal([]byte(evidenceJSON), &c.EvidenceItems); err != nil {
		return Case{}, err
	}
	c.CreatedAt = time.Unix(createdAtUnix, 0).UTC()
	return c, nil
}

func buildOutboxPayload(input IngestInput) ([]byte, error) {
	c := input.Case
	return json.Marshal(map[string]any{
		"event_id":       input.IdempotencyKey,
		"case_id":        c.CaseID,
		"merchant_id":    c.MerchantID,
		"event_type":     c.EventType,
		"risk_score":     c.RiskScore,
		"occurred_at_ms": c.OccurredAtMs,
	})
}

func insertAudit(ctx context.Context, execer sqlExecer, caseID, action string, detail map[string]any) error {
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	_, err = execer.ExecContext(
		ctx,
		"INSERT INTO risk_audit_logs (case_id, action, detail_json) VALUES (?, ?, ?)",
		caseID,
		action,
		string(raw),
	)
	return err
}

func countQuery(ctx context.Context, queryer sqlQueryer, query string) (int64, error) {
	var count int64
	if err := queryer.QueryRowContext(ctx, query).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func cloneAuditLog(entry AuditLog) AuditLog {
	entry.Detail = cloneDetail(entry.Detail)
	return entry
}

func cloneDetail(detail map[string]any) map[string]any {
	if len(detail) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(detail))
	for key, value := range detail {
		cloned[key] = value
	}
	return cloned
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type sqlQueryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}
