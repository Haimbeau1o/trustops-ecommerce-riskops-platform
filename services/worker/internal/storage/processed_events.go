package storage

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// ProcessedEventStore tracks worker-side dedup state for consumed events.
type ProcessedEventStore interface {
	TryStart(ctx context.Context, eventID, caseID, consumerName string) (bool, error)
	MarkProcessed(ctx context.Context, eventID string) error
	MarkFailed(ctx context.Context, eventID, lastError string) error
}

// ProcessedEventRecord stores the dedup state for one consumed event.
type ProcessedEventRecord struct {
	EventID      string
	CaseID       string
	Status       string
	ConsumerName string
	ProcessedAt  time.Time
	LastError    string
}

// InMemoryProcessedEventStore is a local fallback store suitable for tests and demos.
type InMemoryProcessedEventStore struct {
	mu    sync.Mutex
	items map[string]ProcessedEventRecord
}

func NewInMemoryProcessedEventStore() *InMemoryProcessedEventStore {
	return &InMemoryProcessedEventStore{
		items: make(map[string]ProcessedEventRecord),
	}
}

func (s *InMemoryProcessedEventStore) TryStart(_ context.Context, eventID, caseID, consumerName string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.items[eventID]; exists {
		return false, nil
	}
	s.items[eventID] = ProcessedEventRecord{
		EventID:      eventID,
		CaseID:       caseID,
		Status:       "processing",
		ConsumerName: consumerName,
	}
	return true, nil
}

func (s *InMemoryProcessedEventStore) MarkProcessed(_ context.Context, eventID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := s.items[eventID]
	record.Status = "processed"
	record.ProcessedAt = time.Now().UTC()
	s.items[eventID] = record
	return nil
}

func (s *InMemoryProcessedEventStore) MarkFailed(_ context.Context, eventID, lastError string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := s.items[eventID]
	record.Status = "failed"
	record.LastError = lastError
	s.items[eventID] = record
	return nil
}

// MySQLProcessedEventStore persists worker dedup state to MySQL.
type MySQLProcessedEventStore struct {
	db *sql.DB
}

func NewMySQLProcessedEventStore(db *sql.DB) *MySQLProcessedEventStore {
	return &MySQLProcessedEventStore{db: db}
}

func (s *MySQLProcessedEventStore) TryStart(ctx context.Context, eventID, caseID, consumerName string) (bool, error) {
	result, err := s.db.ExecContext(
		ctx,
		"INSERT INTO risk_worker_processed_events (event_id, case_id, status, consumer_name) VALUES (?, ?, 'processing', ?) ON DUPLICATE KEY UPDATE event_id = event_id",
		eventID,
		caseID,
		consumerName,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func (s *MySQLProcessedEventStore) MarkProcessed(ctx context.Context, eventID string) error {
	_, err := s.db.ExecContext(
		ctx,
		"UPDATE risk_worker_processed_events SET status = 'processed', processed_at = CURRENT_TIMESTAMP, last_error = '' WHERE event_id = ?",
		eventID,
	)
	return err
}

func (s *MySQLProcessedEventStore) MarkFailed(ctx context.Context, eventID, lastError string) error {
	_, err := s.db.ExecContext(
		ctx,
		"UPDATE risk_worker_processed_events SET status = 'failed', last_error = ? WHERE event_id = ?",
		lastError,
		eventID,
	)
	return err
}
