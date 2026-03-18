package storage

import (
	"context"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

func TestInMemoryProcessedEventStoreTryStartDeduplicates(t *testing.T) {
	store := NewInMemoryProcessedEventStore()
	ctx := context.Background()

	acquired, err := store.TryStart(ctx, "evt-risk-001", "case-risk-001", "worker-a")
	if err != nil {
		t.Fatalf("TryStart() error = %v", err)
	}
	if !acquired {
		t.Fatalf("expected first TryStart to acquire")
	}

	duplicate, err := store.TryStart(ctx, "evt-risk-001", "case-risk-001", "worker-a")
	if err != nil {
		t.Fatalf("second TryStart() error = %v", err)
	}
	if duplicate {
		t.Fatalf("expected duplicate TryStart to be rejected")
	}
}

func TestInMemoryProcessedEventStoreMarksProcessed(t *testing.T) {
	store := NewInMemoryProcessedEventStore()
	ctx := context.Background()

	if _, err := store.TryStart(ctx, "evt-risk-001", "case-risk-001", "worker-a"); err != nil {
		t.Fatalf("TryStart() error = %v", err)
	}
	if err := store.MarkProcessed(ctx, "evt-risk-001"); err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}

	record := store.items["evt-risk-001"]
	if record.Status != "processed" {
		t.Fatalf("expected status processed, got %q", record.Status)
	}
}

func TestMySQLProcessedEventStoreTryStart(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	store := NewMySQLProcessedEventStore(db)
	mock.ExpectExec("INSERT INTO risk_worker_processed_events").
		WithArgs("evt-risk-001", "case-risk-001", "worker-a").
		WillReturnResult(sqlmock.NewResult(1, 1))

	acquired, err := store.TryStart(context.Background(), "evt-risk-001", "case-risk-001", "worker-a")
	if err != nil {
		t.Fatalf("TryStart() error = %v", err)
	}
	if !acquired {
		t.Fatalf("expected mysql TryStart to acquire")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestMySQLProcessedEventStoreMarkProcessed(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	store := NewMySQLProcessedEventStore(db)
	mock.ExpectExec("UPDATE risk_worker_processed_events SET status = 'processed'").
		WithArgs("evt-risk-001").
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := store.MarkProcessed(context.Background(), "evt-risk-001"); err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}
