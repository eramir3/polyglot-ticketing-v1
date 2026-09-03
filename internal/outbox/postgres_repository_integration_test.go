package outbox

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestPostgresRepositoryClaimPendingReturnsEventsInSequenceOrder(t *testing.T) {
	ctx := context.Background()
	container, err := postgres.Run(
		ctx,
		"postgres:17-alpine",
		postgres.BasicWaitStrategies(),
		postgres.WithDatabase("outbox"),
		postgres.WithUsername("outbox"),
		postgres.WithPassword("outbox-test-password"),
	)
	if err != nil {
		t.Fatalf("start outbox PostgreSQL container: %v", err)
	}
	testcontainers.CleanupContainer(t, container)

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get outbox PostgreSQL connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create outbox database pool: %v", err)
	}
	t.Cleanup(pool.Close)

	if _, err := pool.Exec(ctx, `
		CREATE TABLE outbox_events (
			event_id UUID PRIMARY KEY,
			subject TEXT NOT NULL,
			payload BYTEA NOT NULL,
			created_at TIMESTAMPTZ NOT NULL,
			published_at TIMESTAMPTZ,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT,
			next_attempt_at TIMESTAMPTZ,
			locked_until TIMESTAMPTZ,
			traceparent TEXT,
			tracestate TEXT,
			sequence BIGINT GENERATED ALWAYS AS IDENTITY
		)`); err != nil {
		t.Fatalf("create outbox table: %v", err)
	}

	createdAt := time.Date(2026, time.September, 3, 13, 10, 26, 0, time.UTC)
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for _, event := range []struct {
		id       string
		sequence int64
		payload  string
	}{
		{id: ids[2], sequence: 3, payload: "third"},
		{id: ids[1], sequence: 2, payload: "second"},
		{id: ids[0], sequence: 1, payload: "first"},
	} {
		if _, err := pool.Exec(ctx, `
			INSERT INTO outbox_events (event_id, sequence, subject, payload, created_at)
			OVERRIDING SYSTEM VALUE
			VALUES ($1, $2, $3, $4, $5)`, event.id, event.sequence, "tickets.ticket.updated.v1", []byte(event.payload), createdAt); err != nil {
			t.Fatalf("seed outbox event %d: %v", event.sequence, err)
		}
	}

	events, err := NewPostgresRepository(pool).ClaimPending(ctx, 3, time.Minute)
	if err != nil {
		t.Fatalf("claim pending events: %v", err)
	}
	if len(events) != 3 {
		t.Fatalf("expected three claimed events, got %d", len(events))
	}
	for index, expectedID := range ids {
		if events[index].EventID != expectedID {
			t.Fatalf("expected event %d to be %q, got %q", index, expectedID, events[index].EventID)
		}
	}
}
