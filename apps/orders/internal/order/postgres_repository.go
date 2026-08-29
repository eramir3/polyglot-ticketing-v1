package order

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	orderevents "polyglot-ticketing-v1/contracts/orders"
	"polyglot-ticketing-v1/internal/tracing"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
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

	var projectedTicket Ticket
	err = tx.QueryRow(ctx, `
		SELECT id::text, title, price, aggregate_version
		FROM tickets
		WHERE id = $1
		FOR UPDATE`, input.TicketID).Scan(
		&projectedTicket.ID,
		&projectedTicket.Title,
		&projectedTicket.Price,
		&projectedTicket.AggregateVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReservationResult{}, ErrNotFound
	}
	if err != nil {
		return ReservationResult{}, err
	}

	existing, found, err := findBlockingOrder(ctx, tx, projectedTicket.ID)
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
	if err := insertOrderCreatedEvent(ctx, tx, created, projectedTicket); err != nil {
		return ReservationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ReservationResult{}, err
	}

	return ReservationResult{Created: true, Order: created}, nil
}

func insertOrderCreatedEvent(ctx context.Context, tx pgx.Tx, created Order, ticket Ticket) error {
	occurredAt := time.Now().UTC()
	eventID := uuid.NewString()
	payload, err := marshalOrderCreated(eventID, occurredAt, created, ticket)
	if err != nil {
		return err
	}

	traceparent, tracestate := tracing.HeaderValues(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, subject, payload, created_at, traceparent, tracestate)
		VALUES ($1, $2, $3, $4, $5, $6)`, eventID, orderevents.OrderCreatedSubject, payload, occurredAt, traceparent, tracestate)
	return err
}

func marshalOrderCreated(eventID string, occurredAt time.Time, created Order, ticket Ticket) ([]byte, error) {
	return proto.Marshal(&ordersv1.OrderCreated{
		EventId:          eventID,
		OccurredAt:       timestamppb.New(occurredAt),
		OrderId:          created.ID,
		OrderStatus:      toProtoOrderStatus(created.Status),
		UserId:           created.UserID,
		ExpiresAt:        timestamppb.New(created.ExpiresAt),
		AggregateVersion: created.AggregateVersion,
		Ticket: &ordersv1.OrderTicket{
			Id:    ticket.ID,
			Price: ticket.Price,
		},
	})
}

func toProtoOrderStatus(value Status) ordersv1.OrderStatus {
	switch value {
	case StatusCreated:
		return ordersv1.OrderStatus_ORDER_STATUS_CREATED
	case StatusCanceled:
		return ordersv1.OrderStatus_ORDER_STATUS_CANCELED
	case StatusAwaitingPayment:
		return ordersv1.OrderStatus_ORDER_STATUS_AWAITING_PAYMENT
	case StatusComplete:
		return ordersv1.OrderStatus_ORDER_STATUS_COMPLETE
	default:
		return ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func findBlockingOrder(ctx context.Context, tx pgx.Tx, ticketID string) (Order, bool, error) {
	var found Order
	var status string
	err := tx.QueryRow(ctx, `
		SELECT id, expires_at, user_id, ticket_id, status::text, aggregate_version
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
		&found.AggregateVersion,
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
		RETURNING id, expires_at, user_id, ticket_id, status::text, aggregate_version`,
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
		&created.AggregateVersion,
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
		SELECT id::text, expires_at, user_id, ticket_id::text, status::text, aggregate_version
		FROM orders
		WHERE id = $1 AND user_id = $2`, orderID, userID).Scan(
		&found.ID,
		&found.ExpiresAt,
		&found.UserID,
		&found.TicketID,
		&status,
		&found.AggregateVersion,
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

func (repository *PostgresRepository) CancelByIDAndUser(
	ctx context.Context,
	orderID string,
	userID string,
) (Order, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Order{}, err
	}
	defer tx.Rollback(ctx)

	var canceled Order
	var canceledStatus string
	err = tx.QueryRow(ctx, `
		UPDATE orders
		SET status = $3,
		    aggregate_version = aggregate_version + 1
		WHERE id = $1
		  AND user_id = $2
		  AND status IN ('Created', 'AwaitingPayment')
		RETURNING id::text, expires_at, user_id, ticket_id::text, status::text, aggregate_version`,
		orderID,
		userID,
		StatusCanceled,
	).Scan(
		&canceled.ID,
		&canceled.ExpiresAt,
		&canceled.UserID,
		&canceled.TicketID,
		&canceledStatus,
		&canceled.AggregateVersion,
	)
	if err == nil {
		canceled.Status = Status(canceledStatus)
		if err := insertOrderCanceledEvent(ctx, tx, canceled); err != nil {
			return Order{}, err
		}
		if err := tx.Commit(ctx); err != nil {
			return Order{}, err
		}
		return canceled, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Order{}, err
	}

	var found Order
	var foundStatus string
	err = tx.QueryRow(ctx, `
		SELECT id::text, expires_at, user_id, ticket_id::text, status::text, aggregate_version
		FROM orders
		WHERE id = $1 AND user_id = $2`, orderID, userID).Scan(
		&found.ID,
		&found.ExpiresAt,
		&found.UserID,
		&found.TicketID,
		&foundStatus,
		&found.AggregateVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, err
	}
	found.Status = Status(foundStatus)
	if found.Status == StatusCanceled {
		if err := tx.Commit(ctx); err != nil {
			return Order{}, err
		}
		return found, nil
	}

	return Order{}, ErrOrderNotCancelable
}

// ApplyExpirationComplete records an expiration delivery before conditionally
// canceling a Created order. The state transition and its outbox event share a
// transaction, so redelivery cannot publish a second OrderCanceled event.
func (repository *PostgresRepository) ApplyExpirationComplete(
	ctx context.Context,
	eventID string,
	orderID string,
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
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}

	var canceled Order
	var canceledStatus string
	err = tx.QueryRow(ctx, `
		UPDATE orders
		SET status = $2,
		    aggregate_version = aggregate_version + 1
		WHERE id = $1 AND status = 'Created'
		RETURNING id::text, expires_at, user_id, ticket_id::text, status::text, aggregate_version`,
		orderID,
		StatusCanceled,
	).Scan(
		&canceled.ID,
		&canceled.ExpiresAt,
		&canceled.UserID,
		&canceled.TicketID,
		&canceledStatus,
		&canceled.AggregateVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}

	canceled.Status = Status(canceledStatus)
	if err := insertOrderCanceledEvent(ctx, tx, canceled); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ApplyPaymentCreated records a payment delivery before conditionally moving a
// Created order to AwaitingPayment. Late and duplicate deliveries are no-ops.
func (repository *PostgresRepository) ApplyPaymentCreated(
	ctx context.Context,
	eventID string,
	orderID string,
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
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}

	_, err = tx.Exec(ctx, `
		UPDATE orders
		SET status = $2,
		    aggregate_version = aggregate_version + 1
		WHERE id = $1 AND status = 'Created'`, orderID, StatusAwaitingPayment)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ApplyPaymentSucceeded records a payment success before moving an eligible
// order to Complete. A success may arrive before PaymentCreated, so Created
// orders are eligible alongside AwaitingPayment orders.
func (repository *PostgresRepository) ApplyPaymentSucceeded(
	ctx context.Context,
	eventID string,
	orderID string,
) error {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	processed, err := recordProcessedOrderEvent(ctx, tx, eventID)
	if err != nil || !processed {
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	_, err = tx.Exec(ctx, `
		UPDATE orders
		SET status = $2,
		    aggregate_version = aggregate_version + 1
		WHERE id = $1 AND status IN ('Created', 'AwaitingPayment')`, orderID, StatusComplete)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ApplyPaymentFailed records a payment failure before conditionally canceling
// an eligible order. Its state transition and OrderCanceled outbox event share
// one transaction, so redelivery cannot emit a second cancellation.
func (repository *PostgresRepository) ApplyPaymentFailed(
	ctx context.Context,
	eventID string,
	orderID string,
) error {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	processed, err := recordProcessedOrderEvent(ctx, tx, eventID)
	if err != nil || !processed {
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	var canceled Order
	var canceledStatus string
	err = tx.QueryRow(ctx, `
		UPDATE orders
		SET status = $2,
		    aggregate_version = aggregate_version + 1
		WHERE id = $1 AND status IN ('Created', 'AwaitingPayment')
		RETURNING id::text, expires_at, user_id, ticket_id::text, status::text, aggregate_version`, orderID, StatusCanceled).Scan(
		&canceled.ID,
		&canceled.ExpiresAt,
		&canceled.UserID,
		&canceled.TicketID,
		&canceledStatus,
		&canceled.AggregateVersion,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return tx.Commit(ctx)
	}
	if err != nil {
		return err
	}
	canceled.Status = Status(canceledStatus)
	if err := insertOrderCanceledEvent(ctx, tx, canceled); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func recordProcessedOrderEvent(ctx context.Context, tx pgx.Tx, eventID string) (bool, error) {
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

func insertOrderCanceledEvent(ctx context.Context, tx pgx.Tx, canceled Order) error {
	occurredAt := time.Now().UTC()
	eventID := uuid.NewString()
	payload, err := marshalOrderCanceled(eventID, occurredAt, canceled)
	if err != nil {
		return err
	}

	traceparent, tracestate := tracing.HeaderValues(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, subject, payload, created_at, traceparent, tracestate)
		VALUES ($1, $2, $3, $4, $5, $6)`, eventID, orderevents.OrderCanceledSubject, payload, occurredAt, traceparent, tracestate)
	return err
}

func marshalOrderCanceled(eventID string, occurredAt time.Time, canceled Order) ([]byte, error) {
	return proto.Marshal(&ordersv1.OrderCanceled{
		EventId:          eventID,
		OccurredAt:       timestamppb.New(occurredAt),
		OrderId:          canceled.ID,
		AggregateVersion: canceled.AggregateVersion,
		Ticket: &ordersv1.OrderCanceledTicket{
			Id: canceled.TicketID,
		},
	})
}

func (repository *PostgresRepository) ListByUser(ctx context.Context, userID string) ([]Order, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT id::text, expires_at, user_id, ticket_id::text, status::text, aggregate_version
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
			&found.AggregateVersion,
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

// UpsertTicketFromEvent records the event and applies a contiguous ticket
// snapshot in one transaction. Redelivered or stale event snapshots are
// intentionally no-ops. A future version is rolled back so it can be retried.
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

	var storedVersion int64
	err = tx.QueryRow(ctx, `
		SELECT aggregate_version
		FROM tickets
		WHERE id = $1
		FOR UPDATE`, ticket.ID).Scan(&storedVersion)
	hasStoredVersion := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}

	decision, err := decideTicketEventVersion(hasStoredVersion, storedVersion, ticket.AggregateVersion)
	if err != nil {
		return err
	}
	if decision == ignoreTicketEventVersion {
		return tx.Commit(ctx)
	}

	if !hasStoredVersion {
		_, err = tx.Exec(ctx, `
			INSERT INTO tickets (id, title, price, aggregate_version)
			VALUES ($1, $2, $3, $4)`,
			ticket.ID,
			ticket.Title,
			ticket.Price,
			ticket.AggregateVersion,
		)
	} else {
		_, err = tx.Exec(ctx, `
			UPDATE tickets
			SET title = $1,
			    price = $2,
			    aggregate_version = $3
			WHERE id = $4`,
			ticket.Title,
			ticket.Price,
			ticket.AggregateVersion,
			ticket.ID,
		)
	}
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

type ticketEventVersionDecision uint8

const (
	applyTicketEventVersion ticketEventVersionDecision = iota
	ignoreTicketEventVersion
)

func decideTicketEventVersion(
	hasStoredVersion bool,
	storedVersion int64,
	incomingVersion int64,
) (ticketEventVersionDecision, error) {
	if !hasStoredVersion {
		if incomingVersion == 0 {
			return applyTicketEventVersion, nil
		}
		return 0, ErrTicketEventVersionGap
	}
	if incomingVersion > storedVersion+1 {
		return 0, ErrTicketEventVersionGap
	}
	if incomingVersion <= storedVersion {
		return ignoreTicketEventVersion, nil
	}

	return applyTicketEventVersion, nil
}
