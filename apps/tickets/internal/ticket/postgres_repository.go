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

	"polyglot-ticketing-v1/apps/tickets/internal/outbox"
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
		 RETURNING id, title, price, user_id`,
		input.Title,
		input.Price,
		input.UserID,
	).Scan(&created.ID, &created.Title, &created.Price, &created.UserID)
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
		VALUES ($1, $2, $3, $4)`, eventID, outbox.TicketCreatedSubject, payload, occurredAt)
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
		EventId:    eventID,
		OccurredAt: timestamppb.New(occurredAt),
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
		EventId:    eventID,
		OccurredAt: timestamppb.New(occurredAt),
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
		`SELECT id, title, price, user_id
		 FROM tickets
		 WHERE id = $1`,
		id,
	).Scan(&found.ID, &found.Title, &found.Price, &found.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}

	return found, err
}

func (repository *PostgresRepository) List(ctx context.Context) ([]Ticket, error) {
	rows, err := repository.pool.Query(
		ctx,
		`SELECT id, title, price, user_id
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
		if err := rows.Scan(&listed.ID, &listed.Title, &listed.Price, &listed.UserID); err != nil {
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

	var updated Ticket
	err = tx.QueryRow(
		ctx,
		`UPDATE tickets
		 SET title = $1, price = $2
		 WHERE id = $3 AND user_id = $4
		 RETURNING id, title, price, user_id`,
		input.Title,
		input.Price,
		id,
		input.UserID,
	).Scan(&updated.ID, &updated.Title, &updated.Price, &updated.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Ticket{}, ErrNotFound
	}
	if err != nil {
		return Ticket{}, err
	}

	occurredAt := time.Now().UTC()
	eventID := uuid.NewString()
	payload, err := marshalTicketUpdated(eventID, occurredAt, updated)
	if err != nil {
		return Ticket{}, err
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, subject, payload, created_at)
		VALUES ($1, $2, $3, $4)`, eventID, outbox.TicketUpdatedSubject, payload, occurredAt)
	if err != nil {
		return Ticket{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Ticket{}, err
	}

	return updated, nil
}
