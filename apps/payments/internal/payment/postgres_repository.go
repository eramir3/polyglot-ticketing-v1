package payment

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	paymentevents "polyglot-ticketing-v1/contracts/payments"
	"polyglot-ticketing-v1/internal/tracing"
	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (repository *PostgresRepository) Create(
	ctx context.Context,
	input CreateInput,
) (Payment, bool, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Payment{}, false, err
	}
	defer tx.Rollback(ctx)

	var userID string
	var orderStatus string
	err = tx.QueryRow(ctx, `
		SELECT user_id, status::text
		FROM orders
		WHERE id = $1
		FOR UPDATE`, input.OrderID).Scan(&userID, &orderStatus)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && userID != input.UserID) {
		return Payment{}, false, ErrOrderNotFound
	}
	if err != nil {
		return Payment{}, false, err
	}
	var existing Payment
	err = tx.QueryRow(ctx, `
		SELECT id::text, order_id::text, status::text, traceparent, tracestate
		FROM payments
		WHERE order_id = $1`, input.OrderID).Scan(&existing.ID, &existing.OrderID, &existing.Status)
	if err == nil {
		if err := tx.Commit(ctx); err != nil {
			return Payment{}, false, err
		}
		return existing, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, false, err
	}
	if OrderStatus(orderStatus) != OrderStatusCreated {
		return Payment{}, false, ErrOrderNotPayable
	}

	var created Payment
	traceparent, tracestate := tracing.HeaderValues(ctx)
	err = tx.QueryRow(ctx, `
		INSERT INTO payments (order_id, traceparent, tracestate)
		VALUES ($1, $2, $3)
		RETURNING id::text, order_id::text, status::text`, input.OrderID, traceparent, tracestate).Scan(&created.ID, &created.OrderID, &created.Status)
	if err != nil {
		return Payment{}, false, err
	}
	if err := insertPaymentCreatedEvent(ctx, tx, created); err != nil {
		return Payment{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payment{}, false, err
	}
	return created, true, nil
}

func insertPaymentCreatedEvent(ctx context.Context, tx pgx.Tx, created Payment) error {
	occurredAt := time.Now().UTC()
	eventID := uuid.NewString()
	payload, err := proto.Marshal(&paymentsv1.PaymentCreated{
		EventId:    eventID,
		OccurredAt: timestamppb.New(occurredAt),
		PaymentId:  created.ID,
		OrderId:    created.OrderID,
	})
	if err != nil {
		return err
	}

	traceparent, tracestate := tracing.HeaderValues(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, subject, payload, created_at, traceparent, tracestate)
		VALUES ($1, $2, $3, $4, $5, $6)`, eventID, paymentevents.PaymentCreatedSubject, payload, occurredAt, traceparent, tracestate)
	return err
}

// ResolveNextPending claims and resolves one payment. The status update and
// result event use one transaction, leaving a failed transaction Pending for a
// later processor retry.
func (repository *PostgresRepository) ResolveNextPending(ctx context.Context, outcome ProcessorOutcome) (Payment, bool, error) {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Payment{}, false, err
	}
	defer tx.Rollback(ctx)

	var pending Payment
	err = tx.QueryRow(ctx, `
		SELECT id::text, order_id::text, status::text
		FROM payments
		WHERE status = $1
		ORDER BY id
		FOR UPDATE SKIP LOCKED
		LIMIT 1`, StatusPending).Scan(&pending.ID, &pending.OrderID, &pending.Status, &pending.Traceparent, &pending.Tracestate)
	if errors.Is(err, pgx.ErrNoRows) {
		return Payment{}, false, tx.Commit(ctx)
	}
	if err != nil {
		return Payment{}, false, err
	}
	ctx = tracing.ContextFromHeaders(ctx, pending.Traceparent, pending.Tracestate)

	resolvedStatus, err := statusForOutcome(outcome)
	if err != nil {
		return Payment{}, false, err
	}
	_, err = tx.Exec(ctx, `
		UPDATE payments
		SET status = $2
		WHERE id = $1`, pending.ID, resolvedStatus)
	if err != nil {
		return Payment{}, false, err
	}
	pending.Status = resolvedStatus
	if err := insertPaymentResultEvent(ctx, tx, pending); err != nil {
		return Payment{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Payment{}, false, err
	}
	return pending, true, nil
}

func statusForOutcome(outcome ProcessorOutcome) (Status, error) {
	switch outcome {
	case ProcessorOutcomeSuccess:
		return StatusSucceeded, nil
	case ProcessorOutcomeFailure:
		return StatusFailed, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidOutcome, outcome)
	}
}

func insertPaymentResultEvent(ctx context.Context, tx pgx.Tx, resolved Payment) error {
	occurredAt := time.Now().UTC()
	eventID := uuid.NewString()
	var (
		payload []byte
		err     error
		subject string
	)
	switch resolved.Status {
	case StatusSucceeded:
		subject = paymentevents.PaymentSucceededSubject
		payload, err = proto.Marshal(&paymentsv1.PaymentSucceeded{
			EventId: eventID, OccurredAt: timestamppb.New(occurredAt), PaymentId: resolved.ID, OrderId: resolved.OrderID,
		})
	case StatusFailed:
		subject = paymentevents.PaymentFailedSubject
		payload, err = proto.Marshal(&paymentsv1.PaymentFailed{
			EventId: eventID, OccurredAt: timestamppb.New(occurredAt), PaymentId: resolved.ID, OrderId: resolved.OrderID,
		})
	default:
		return fmt.Errorf("cannot publish result for payment status %q", resolved.Status)
	}
	if err != nil {
		return err
	}

	traceparent, tracestate := tracing.HeaderValues(ctx)
	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, subject, payload, created_at, traceparent, tracestate)
		VALUES ($1, $2, $3, $4, $5, $6)`, eventID, subject, payload, occurredAt, traceparent, tracestate)
	return err
}

// UpsertOrderFromEvent records a contiguous OrderCreated snapshot in the
// Payments-owned projection. Duplicate and stale snapshots are no-ops.
func (repository *PostgresRepository) UpsertOrderFromEvent(
	ctx context.Context,
	eventID string,
	incoming Order,
) error {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	processed, err := recordProcessedEvent(ctx, tx, eventID)
	if err != nil || !processed {
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	var storedVersion int64
	err = tx.QueryRow(ctx, `
		SELECT aggregate_version
		FROM orders
		WHERE id = $1
		FOR UPDATE`, incoming.ID).Scan(&storedVersion)
	hasStoredOrder := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err := validateOrderVersion(hasStoredOrder, storedVersion, incoming.AggregateVersion); err != nil {
		return err
	}
	if hasStoredOrder {
		return tx.Commit(ctx)
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO orders (id, aggregate_version, user_id, price, status)
		VALUES ($1, $2, $3, $4, $5)`,
		incoming.ID,
		incoming.AggregateVersion,
		incoming.UserID,
		incoming.Price,
		incoming.Status,
	)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (repository *PostgresRepository) CancelOrderFromEvent(
	ctx context.Context,
	eventID string,
	orderID string,
	aggregateVersion int64,
) error {
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	processed, err := recordProcessedEvent(ctx, tx, eventID)
	if err != nil || !processed {
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	}

	var storedVersion int64
	err = tx.QueryRow(ctx, `
		SELECT aggregate_version
		FROM orders
		WHERE id = $1
		FOR UPDATE`, orderID).Scan(&storedVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderEventVersionGap
	}
	if err != nil {
		return err
	}
	if err := validateOrderVersion(true, storedVersion, aggregateVersion); err != nil {
		return err
	}
	if aggregateVersion <= storedVersion {
		return tx.Commit(ctx)
	}

	_, err = tx.Exec(ctx, `
		UPDATE orders
		SET status = $1, aggregate_version = $2
		WHERE id = $3`, OrderStatusCanceled, aggregateVersion, orderID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func validateOrderVersion(hasStoredOrder bool, storedVersion, incomingVersion int64) error {
	if !hasStoredOrder {
		if incomingVersion != 0 {
			return ErrOrderEventVersionGap
		}
		return nil
	}
	if incomingVersion > storedVersion+1 {
		return ErrOrderEventVersionGap
	}
	return nil
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

var _ Repository = (*PostgresRepository)(nil)
var _ OrderProjectionRepository = (*PostgresRepository)(nil)
