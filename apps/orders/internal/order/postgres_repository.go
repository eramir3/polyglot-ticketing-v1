package order

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// UpsertTicketFromEvent records the event and applies the ticket projection in
// one transaction. Redelivered event IDs are intentionally no-ops.
func (repository *PostgresRepository) UpsertTicketFromEvent(
	ctx context.Context,
	eventID string,
	ticket Ticket,
) error {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var insertedEventID string
	err = tx.QueryRow(ctx, `
		INSERT INTO processed_events (event_id)
		VALUES ($1)
		ON CONFLICT DO NOTHING
		RETURNING event_id::text`, eventID).Scan(&insertedEventID)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}
	if err == pgx.ErrNoRows {
		return tx.Commit(ctx)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO tickets (id, title, price)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE
		SET title = EXCLUDED.title, price = EXCLUDED.price`,
		ticket.ID,
		ticket.Title,
		ticket.Price,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
