package payment

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	paymentevents "polyglot-ticketing-v1/contracts/payments"
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
		SELECT id::text, order_id::text
		FROM payments
		WHERE order_id = $1`, input.OrderID).Scan(&existing.ID, &existing.OrderID)
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
	err = tx.QueryRow(ctx, `
		INSERT INTO payments (order_id)
		VALUES ($1)
		RETURNING id::text, order_id::text`, input.OrderID).Scan(&created.ID, &created.OrderID)
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

	_, err = tx.Exec(ctx, `
		INSERT INTO outbox_events (event_id, subject, payload, created_at)
		VALUES ($1, $2, $3, $4)`, eventID, paymentevents.PaymentCreatedSubject, payload, occurredAt)
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
