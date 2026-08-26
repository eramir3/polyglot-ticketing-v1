package outbox

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (repository *PostgresRepository) ClaimPending(
	ctx context.Context,
	limit int,
	leaseDuration time.Duration,
) ([]Event, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		WITH pending AS (
			SELECT event_id
			FROM outbox_events
			WHERE published_at IS NULL
			  AND (locked_until IS NULL OR locked_until < NOW())
			  AND (next_attempt_at IS NULL OR next_attempt_at <= NOW())
			ORDER BY created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE outbox_events
		SET locked_until = NOW() + $2::interval
		FROM pending
		WHERE outbox_events.event_id = pending.event_id
		RETURNING outbox_events.event_id::text, outbox_events.subject, outbox_events.payload`,
		limit,
		leaseDuration.String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]Event, 0, limit)
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.EventID, &event.Subject, &event.Payload); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return events, nil
}

func (repository *PostgresRepository) MarkPublished(ctx context.Context, eventID string) error {
	_, err := repository.pool.Exec(ctx, `
		UPDATE outbox_events
		SET published_at = NOW(), locked_until = NULL, last_error = NULL, next_attempt_at = NULL
		WHERE event_id = $1`, eventID)
	return err
}

func (repository *PostgresRepository) MarkFailed(ctx context.Context, eventID string, publishError error) error {
	_, err := repository.pool.Exec(ctx, `
		UPDATE outbox_events
		SET attempts = attempts + 1,
			last_error = $2,
			locked_until = NULL,
			next_attempt_at = NOW() + make_interval(
				secs => LEAST((2 ^ LEAST(attempts + 1, 6))::integer, 60)
			)
		WHERE event_id = $1`, eventID, publishError.Error())
	return err
}
