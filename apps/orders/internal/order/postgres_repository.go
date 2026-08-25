package order

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

// Create inserts an order only when the referenced ticket is available in the
// orders-owned projection. The INSERT ... SELECT keeps that check and insert
// in one database operation.
func (repository *PostgresRepository) Create(ctx context.Context, input CreateInput) (Order, error) {
	var created Order
	var status string
	err := repository.pool.QueryRow(ctx, `
		INSERT INTO orders (expires_at, user_id, ticket_id, status)
		SELECT $1, $2, tickets.id, $3
		FROM tickets
		WHERE tickets.id = $4
		RETURNING id, expires_at, user_id, ticket_id, status::text`,
		input.ExpiresAt,
		input.UserID,
		StatusCreated,
		input.TicketID,
	).Scan(
		&created.ID,
		&created.ExpiresAt,
		&created.UserID,
		&created.TicketID,
		&status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, err
	}
	created.Status = Status(status)

	return created, nil
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
