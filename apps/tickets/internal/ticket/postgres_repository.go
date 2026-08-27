package ticket

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	ticketevents "polyglot-ticketing-v1/contracts/tickets"
	ticketsv1 "polyglot-ticketing-v1/protogen/go/tickets/v1"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (repository *PostgresRepository) Create(ctx context.Context, input CreateInput) (Ticket, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Ticket{}, err
	}
	defer tx.Rollback(ctx)

	var created Ticket
	err = tx.QueryRow(
		ctx,
		`INSERT INTO tickets (title, price, user_id)
		 VALUES ($1, $2, $3)
		RETURNING id, title, price, user_id, COALESCE(reserved_by_order_id::text, ''), aggregate_version`,
		input.Title,
		input.Price,
		input.UserID,
	).Scan(&created.ID, &created.Title, &created.Price, &created.UserID, &created.ReservedByOrderID, &created.AggregateVersion)
	if err != nil {
		return Ticket{}, err
	}

	occurredAt := time.Now().UTC()
	eventID := uuid.NewString()
	payload, err := marshalTicketCreated(eventID, occurredAt, created)
	if err != nil {
		return Ticket{}, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, subject, payload, created_at)
		VALUES ($1, $2, $3, $4)`, eventID, ticketevents.TicketCreatedSubject, payload, occurredAt)
	if err != nil {
		return Ticket{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Ticket{}, err
	}

	return created, nil
}

func marshalTicketCreated(eventID string, occurredAt time.Time, created Ticket) ([]byte, error) {
	return proto.Marshal(&ticketsv1.TicketCreated{
		EventId:          eventID,
		OccurredAt:       timestamppb.New(occurredAt),
		AggregateVersion: created.AggregateVersion,
		Ticket: &ticketsv1.Ticket{
			Id:     created.ID,
			Title:  created.Title,
			Price:  created.Price,
			UserId: created.UserID,
		},
	})
}

func marshalTicketUpdated(eventID string, occurredAt time.Time, updated Ticket) ([]byte, error) {
	return proto.Marshal(&ticketsv1.TicketUpdated{
		EventId:          eventID,
		OccurredAt:       timestamppb.New(occurredAt),
		AggregateVersion: updated.AggregateVersion,
		Ticket: &ticketsv1.Ticket{
			Id:     updated.ID,
			Title:  updated.Title,
			Price:  updated.Price,
			UserId: updated.UserID,
		},
	})
}

func (repository *PostgresRepository) FindByID(ctx context.Context, id string) (Ticket, error) {
	var found Ticket
	err := repository.pool.QueryRow(
		ctx,
		`SELECT id, title, price, user_id, COALESCE(reserved_by_order_id::text, ''), aggregate_version
		 FROM tickets
		 WHERE id = $1`,
		id,
	).Scan(&found.ID, &found.Title, &found.Price, &found.UserID, &found.ReservedByOrderID, &found.AggregateVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}

	return found, err
}

func (repository *PostgresRepository) List(ctx context.Context) ([]Ticket, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT id, title, price, user_id, COALESCE(reserved_by_order_id::text, ''), aggregate_version
		 FROM tickets
		 ORDER BY title ASC, id ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tickets := make([]Ticket, 0)
	for rows.Next() {
		var listed Ticket
		if err := rows.Scan(&listed.ID, &listed.Title, &listed.Price, &listed.UserID, &listed.ReservedByOrderID, &listed.AggregateVersion); err != nil {
			return nil, err
		}

		tickets = append(tickets, listed)
	}

	return tickets, rows.Err()
}

func (repository *PostgresRepository) Update(ctx context.Context, id string, input UpdateInput) (Ticket, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Ticket{}, err
	}
	defer tx.Rollback(ctx)

	var reserved bool
	err = tx.QueryRow(ctx, `
		SELECT reserved_by_order_id IS NOT NULL
		FROM tickets
		WHERE id = $1 AND user_id = $2
		FOR UPDATE`, id, input.UserID).Scan(&reserved)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, err
	}
	if reserved {
		return Ticket{}, ErrReserved
	}

	var updated Ticket
	err = tx.QueryRow(
		ctx,
		`UPDATE tickets
		 SET title = $1, price = $2, aggregate_version = aggregate_version + 1
		 WHERE id = $3 AND user_id = $4
		 RETURNING id, title, price, user_id, COALESCE(reserved_by_order_id::text, ''), aggregate_version`,
		input.Title,
		input.Price,
		id,
		input.UserID,
	).Scan(&updated.ID, &updated.Title, &updated.Price, &updated.UserID, &updated.ReservedByOrderID, &updated.AggregateVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, err
	}

	if err := insertTicketUpdatedEvent(ctx, tx, updated); err != nil {
		return Ticket{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Ticket{}, err
	}

	return updated, nil
}

// ReserveTicketFromOrder records the created event, locks the ticket, and
// publishes its updated snapshot in one transaction.
func (repository *PostgresRepository) ReserveTicketFromOrder(
	ctx context.Context,
	eventID string,
	orderID string,
	ticketID string,
) error {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	processed, err := recordProcessedEvent(ctx, tx, eventID)
	if err != nil {
		return err
	}
	if !processed {
		return tx.Commit(ctx)
	}

	var reservedByOrderID *string
	err = tx.QueryRow(ctx, `
		SELECT reserved_by_order_id::text
		FROM tickets
		WHERE id = $1
		FOR UPDATE`, ticketID).Scan(&reservedByOrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if reservedByOrderID != nil {
		if *reservedByOrderID == orderID {
			return tx.Commit(ctx)
		}
		return ErrReserved
	}

	var updated Ticket
	err = tx.QueryRow(ctx, `
		UPDATE tickets
		SET reserved_by_order_id = $2,
		    aggregate_version = aggregate_version + 1
		WHERE id = $1
		RETURNING id, title, price, user_id, COALESCE(reserved_by_order_id::text, ''), aggregate_version`,
		ticketID,
		orderID,
	).Scan(
		&updated.ID,
		&updated.Title,
		&updated.Price,
		&updated.UserID,
		&updated.ReservedByOrderID,
		&updated.AggregateVersion,
	)
	if err != nil {
		return err
	}
	if err := insertTicketUpdatedEvent(ctx, tx, updated); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// UnreserveTicketFromOrder clears the ticket marker only when the canceled
// order still owns that reservation.
func (repository *PostgresRepository) UnreserveTicketFromOrder(
	ctx context.Context,
	eventID string,
	orderID string,
	ticketID string,
) error {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	processed, err := recordProcessedEvent(ctx, tx, eventID)
	if err != nil {
		return err
	}
	if !processed {
		return tx.Commit(ctx)
	}

	var reservedByOrderID *string
	err = tx.QueryRow(ctx, `
		SELECT reserved_by_order_id::text
		FROM tickets
		WHERE id = $1
		FOR UPDATE`, ticketID).Scan(&reservedByOrderID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if reservedByOrderID == nil || *reservedByOrderID != orderID {
		return ErrOrderReservationPending
	}

	var updated Ticket
	err = tx.QueryRow(ctx, `
		UPDATE tickets
		SET reserved_by_order_id = NULL,
		    aggregate_version = aggregate_version + 1
		WHERE id = $1 AND reserved_by_order_id = $2
		RETURNING id, title, price, user_id, COALESCE(reserved_by_order_id::text, ''), aggregate_version`,
		ticketID,
		orderID,
	).Scan(
		&updated.ID,
		&updated.Title,
		&updated.Price,
		&updated.UserID,
		&updated.ReservedByOrderID,
		&updated.AggregateVersion,
	)
	if err != nil {
		return err
	}
	if err := insertTicketUpdatedEvent(ctx, tx, updated); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func insertTicketUpdatedEvent(ctx context.Context, tx pgx.Tx, updated Ticket) error {
	occurredAt := time.Now().UTC()
	eventID := uuid.NewString()
	payload, err := marshalTicketUpdated(eventID, occurredAt, updated)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, subject, payload, created_at)
		VALUES ($1, $2, $3, $4)`, eventID, ticketevents.TicketUpdatedSubject, payload, occurredAt)
	return err
}

func recordProcessedEvent(ctx context.Context, tx pgx.Tx, eventID string) (bool, error) {
	var insertedEventID string
	err := tx.QueryRow(ctx, `
		INSERT INTO processed_events (event_id)
		VALUES ($1)
		ON CONFLICT DO NOTHING
		RETURNING event_id::text`, eventID).Scan(&insertedEventID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
