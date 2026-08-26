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

// ReserveTicket serializes reservations for a ticket by locking its orders-owned
// projection row. This prevents concurrent callers from both inserting an
// active order for the same ticket.
func (repository *PostgresRepository) ReserveTicket(ctx context.Context, input TicketReservationInput) (ReservationResult, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ReservationResult{}, err
	}
	defer tx.Rollback(ctx)

	var ticketID string
	err = tx.QueryRow(ctx, `SELECT id::text FROM tickets WHERE id = $1 FOR UPDATE`, input.TicketID).Scan(&ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReservationResult{}, ErrNotFound
	}
	if err != nil {
		return ReservationResult{}, err
	}

	existing, found, err := findBlockingOrder(ctx, tx, ticketID)
	if err != nil {
		return ReservationResult{}, err
	}
	if found {
		if existing.UserID == input.UserID && existing.Status != StatusComplete {
			if err := tx.Commit(ctx); err != nil {
				return ReservationResult{}, err
			}
			return ReservationResult{Order: existing}, nil
		}

		return ReservationResult{}, ErrReserved
	}

	created, err := insertOrder(ctx, tx, input)
	if err != nil {
		return ReservationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReservationResult{}, err
	}

	return ReservationResult{Created: true, Order: created}, nil
}

func findBlockingOrder(ctx context.Context, tx pgx.Tx, ticketID string) (Order, bool, error) {
	var found Order
	var status string
	err := tx.QueryRow(ctx, `
		SELECT id, expires_at, user_id, ticket_id, status::text
		FROM orders
		WHERE ticket_id = $1
		  AND (
			status = 'Complete'
			OR (status IN ('Created', 'AwaitingPayment') AND expires_at > NOW())
		  )
		ORDER BY CASE WHEN status = 'Complete' THEN 0 ELSE 1 END, expires_at DESC
		LIMIT 1`, ticketID).Scan(
		&found.ID,
		&found.ExpiresAt,
		&found.UserID,
		&found.TicketID,
		&status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, false, nil
	}
	if err != nil {
		return Order{}, false, err
	}
	found.Status = Status(status)

	return found, true, nil
}

func insertOrder(ctx context.Context, tx pgx.Tx, input TicketReservationInput) (Order, error) {
	var created Order
	var status string
	err := tx.QueryRow(ctx, `
		INSERT INTO orders (expires_at, user_id, ticket_id, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, expires_at, user_id, ticket_id, status::text`,
		input.ExpiresAt,
		input.UserID,
		input.TicketID,
		StatusCreated,
	).Scan(
		&created.ID,
		&created.ExpiresAt,
		&created.UserID,
		&created.TicketID,
		&status,
	)
	if err != nil {
		return Order{}, err
	}
	created.Status = Status(status)

	return created, nil
}

func (repository *PostgresRepository) GetByIDAndUser(
	ctx context.Context,
	orderID string,
	userID string,
) (Order, error) {
	var found Order
	var status string
	err := repository.pool.QueryRow(ctx, `
		SELECT id::text, expires_at, user_id, ticket_id::text, status::text
		FROM orders
		WHERE id = $1 AND user_id = $2`, orderID, userID).Scan(
		&found.ID,
		&found.ExpiresAt,
		&found.UserID,
		&found.TicketID,
		&status,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, err
	}
	found.Status = Status(status)

	return found, nil
}

func (repository *PostgresRepository) ListByUser(ctx context.Context, userID string) ([]Order, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT id::text, expires_at, user_id, ticket_id::text, status::text
		FROM orders
		WHERE user_id = $1
		ORDER BY expires_at DESC, id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	orders := make([]Order, 0)
	for rows.Next() {
		var found Order
		var status string
		if err := rows.Scan(
			&found.ID,
			&found.ExpiresAt,
			&found.UserID,
			&found.TicketID,
			&status,
		); err != nil {
			return nil, err
		}
		found.Status = Status(status)
		orders = append(orders, found)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return orders, nil
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
