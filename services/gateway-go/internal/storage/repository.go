package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var ErrCaseNotFound = errors.New("case_not_found")

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

// Repository defines the storage abstraction for risk cases.
type Repository interface {
	CreateCase(ctx context.Context, c Case) error
	GetCase(ctx context.Context, caseID string) (Case, error)
}

// CaseCache defines cache operations for case lookup.
type CaseCache interface {
	GetCase(ctx context.Context, caseID string) (Case, bool, error)
	SetCase(ctx context.Context, c Case, ttl time.Duration) error
}

// InMemoryRepository is an in-process fallback repository.
type InMemoryRepository struct {
	mu    sync.RWMutex
	cases map[string]Case
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		cases: make(map[string]Case),
	}
}

func (r *InMemoryRepository) CreateCase(_ context.Context, c Case) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}
	r.cases[c.CaseID] = c
	return nil
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

// MySQLRepository persists cases to MySQL via database/sql.
type MySQLRepository struct {
	db *sql.DB
}

func NewMySQLRepository(db *sql.DB) *MySQLRepository {
	return &MySQLRepository{db: db}
}

func (r *MySQLRepository) CreateCase(ctx context.Context, c Case) error {
	evidenceJSON, err := json.Marshal(c.EvidenceItems)
	if err != nil {
		return err
	}

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now().UTC()
	}

	const insertSQL = "INSERT INTO risk_cases (case_id, merchant_id, event_type, risk_category, case_status, risk_score, evidence_json, triggered_by, occurred_at_ms) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)"
	_, err = r.db.ExecContext(
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

func (r *MySQLRepository) GetCase(ctx context.Context, caseID string) (Case, error) {
	const selectSQL = "SELECT case_id, merchant_id, event_type, risk_category, case_status, risk_score, evidence_json, triggered_by, occurred_at_ms, UNIX_TIMESTAMP(created_at) AS created_at_unix FROM risk_cases WHERE case_id = ?"

	var c Case
	var evidenceJSON string
	var createdAtUnix int64
	err := r.db.QueryRowContext(ctx, selectSQL, caseID).Scan(
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
