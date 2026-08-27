package payment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/proto"

	paymentevents "polyglot-ticketing-v1/contracts/payments"
	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

func TestPostgresRepositoryCreatesOnePaymentForEligibleOrder(t *testing.T) {
	ctx := context.Background()
	pool := startPaymentsPostgres(t, ctx)
	repository := NewPostgresRepository(pool)
	orderID := seedProjectedOrder(t, ctx, pool, "user-1", OrderStatusCreated)

	created, wasCreated, err := repository.Create(ctx, CreateInput{OrderID: orderID, UserID: "user-1"})
	if err != nil || !wasCreated || created.OrderID != orderID || created.ID == "" {
		t.Fatalf("create payment: payment=%+v created=%t err=%v", created, wasCreated, err)
	}
	assertPaymentCreatedOutboxEvent(t, ctx, pool, created)

	existing, wasCreated, err := repository.Create(ctx, CreateInput{OrderID: orderID, UserID: "user-1"})
	if err != nil || wasCreated || existing != created {
		t.Fatalf("repeat payment: payment=%+v created=%t err=%v", existing, wasCreated, err)
	}
	var eventCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events`).Scan(&eventCount); err != nil || eventCount != 1 {
		t.Fatalf("expected one outbox event, count=%d err=%v", eventCount, err)
	}
}

func TestPostgresRepositoryReturnsExistingPaymentAfterOrderStartsAwaitingPayment(t *testing.T) {
	ctx := context.Background()
	pool := startPaymentsPostgres(t, ctx)
	repository := NewPostgresRepository(pool)
	orderID := seedProjectedOrder(t, ctx, pool, "user-1", OrderStatusCreated)

	created, wasCreated, err := repository.Create(ctx, CreateInput{OrderID: orderID, UserID: "user-1"})
	if err != nil || !wasCreated {
		t.Fatalf("create payment: payment=%+v created=%t err=%v", created, wasCreated, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE orders SET status = 'AwaitingPayment' WHERE id = $1`, orderID); err != nil {
		t.Fatalf("transition projected order: %v", err)
	}

	existing, wasCreated, err := repository.Create(ctx, CreateInput{OrderID: orderID, UserID: "user-1"})
	if err != nil || wasCreated || existing != created {
		t.Fatalf("repeat payment: payment=%+v created=%t err=%v", existing, wasCreated, err)
	}
}

func TestPostgresRepositoryResolvesPendingPaymentAndWritesResultEvent(t *testing.T) {
	testCases := []struct {
		name            string
		outcome         ProcessorOutcome
		expectedStatus  Status
		expectedSubject string
	}{
		{name: "success", outcome: ProcessorOutcomeSuccess, expectedStatus: StatusSucceeded, expectedSubject: paymentevents.PaymentSucceededSubject},
		{name: "failure", outcome: ProcessorOutcomeFailure, expectedStatus: StatusFailed, expectedSubject: paymentevents.PaymentFailedSubject},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := context.Background()
			pool := startPaymentsPostgres(t, ctx)
			repository := NewPostgresRepository(pool)
			orderID := seedProjectedOrder(t, ctx, pool, "user-1", OrderStatusCreated)
			created, wasCreated, err := repository.Create(ctx, CreateInput{OrderID: orderID, UserID: "user-1"})
			if err != nil || !wasCreated || created.Status != StatusPending {
				t.Fatalf("create pending payment: payment=%+v created=%t err=%v", created, wasCreated, err)
			}

			resolved, err := repository.ResolveNextPending(ctx, testCase.outcome)
			if err != nil || !resolved {
				t.Fatalf("resolve pending payment: resolved=%t err=%v", resolved, err)
			}
			assertPaymentResult(t, ctx, pool, created, testCase.expectedStatus, testCase.expectedSubject)

			resolved, err = repository.ResolveNextPending(ctx, testCase.outcome)
			if err != nil || resolved {
				t.Fatalf("repeat resolution: resolved=%t err=%v", resolved, err)
			}
			var eventCount int
			if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events`).Scan(&eventCount); err != nil || eventCount != 2 {
				t.Fatalf("expected created and one result outbox event, count=%d err=%v", eventCount, err)
			}
		})
	}
}

func TestPostgresRepositoryResolvesPendingPaymentOnlyOnceConcurrently(t *testing.T) {
	ctx := context.Background()
	pool := startPaymentsPostgres(t, ctx)
	repository := NewPostgresRepository(pool)
	orderID := seedProjectedOrder(t, ctx, pool, "user-1", OrderStatusCreated)
	if _, wasCreated, err := repository.Create(ctx, CreateInput{OrderID: orderID, UserID: "user-1"}); err != nil || !wasCreated {
		t.Fatalf("create pending payment: created=%t err=%v", wasCreated, err)
	}

	start := make(chan struct{})
	results := make(chan bool, 2)
	errors := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			resolved, err := repository.ResolveNextPending(ctx, ProcessorOutcomeSuccess)
			results <- resolved
			errors <- err
		}()
	}
	close(start)
	workers.Wait()
	close(results)
	close(errors)

	resolvedCount := 0
	for resolved := range results {
		if resolved {
			resolvedCount++
		}
	}
	for err := range errors {
		if err != nil {
			t.Fatalf("resolve pending payment: %v", err)
		}
	}
	if resolvedCount != 1 {
		t.Fatalf("expected one resolver to claim payment, got %d", resolvedCount)
	}
	var resultEventCount int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM outbox_events WHERE subject = $1`, paymentevents.PaymentSucceededSubject).Scan(&resultEventCount); err != nil || resultEventCount != 1 {
		t.Fatalf("expected one success outbox event, count=%d err=%v", resultEventCount, err)
	}
}

func TestPostgresRepositoryHidesUnownedOrder(t *testing.T) {
	ctx := context.Background()
	pool := startPaymentsPostgres(t, ctx)
	orderID := seedProjectedOrder(t, ctx, pool, "owner-1", OrderStatusCreated)

	_, _, err := NewPostgresRepository(pool).Create(ctx, CreateInput{OrderID: orderID, UserID: "other-user"})

	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("expected hidden order error, got %v", err)
	}
}

func TestPostgresRepositoryRejectsNonPayableOrder(t *testing.T) {
	ctx := context.Background()
	pool := startPaymentsPostgres(t, ctx)
	orderID := seedProjectedOrder(t, ctx, pool, "user-1", OrderStatusCanceled)

	_, _, err := NewPostgresRepository(pool).Create(ctx, CreateInput{OrderID: orderID, UserID: "user-1"})

	if !errors.Is(err, ErrOrderNotPayable) {
		t.Fatalf("expected order cannot be paid error, got %v", err)
	}
}

func startPaymentsPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	container, err := postgres.Run(
		ctx,
		"postgres:17-alpine",
		postgres.BasicWaitStrategies(),
		postgres.WithDatabase("payments"),
		postgres.WithUsername("payments"),
		postgres.WithPassword("payments-test-password"),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start payments PostgreSQL container: %v", err)
	}
	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get payments PostgreSQL connection string: %v", err)
	}

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get test working directory: %v", err)
	}
	migrationsPath := filepath.Join(workingDirectory, "..", "..", "migrations")
	migrator, err := migrate.New(fmt.Sprintf("file://%s", migrationsPath), databaseURL)
	if err != nil {
		t.Fatalf("initialize payments migrations: %v", err)
	}
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("apply payments migrations: %v", err)
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create payments database pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedProjectedOrder(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	userID string,
	status OrderStatus,
) string {
	t.Helper()
	orderID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO orders (id, aggregate_version, user_id, price, status)
		VALUES ($1, $2, $3, $4, $5)`, orderID, 0, userID, 10_000, status); err != nil {
		t.Fatalf("seed projected order: %v", err)
	}
	return orderID
}

func assertPaymentCreatedOutboxEvent(t *testing.T, ctx context.Context, pool *pgxpool.Pool, payment Payment) {
	t.Helper()
	var subject string
	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT subject, payload FROM outbox_events`).Scan(&subject, &payload); err != nil {
		t.Fatalf("read payment-created outbox event: %v", err)
	}
	if subject != paymentevents.PaymentCreatedSubject {
		t.Fatalf("expected subject %q, got %q", paymentevents.PaymentCreatedSubject, subject)
	}
	var event paymentsv1.PaymentCreated
	if err := proto.Unmarshal(payload, &event); err != nil {
		t.Fatalf("unmarshal payment-created event: %v", err)
	}
	if event.GetEventId() == "" || event.GetOccurredAt() == nil || event.GetPaymentId() != payment.ID || event.GetOrderId() != payment.OrderID {
		t.Fatalf("unexpected payment-created event: %+v", &event)
	}
}

func assertPaymentResult(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	payment Payment,
	expectedStatus Status,
	expectedSubject string,
) {
	t.Helper()
	var status string
	if err := pool.QueryRow(ctx, `SELECT status::text FROM payments WHERE id = $1`, payment.ID).Scan(&status); err != nil || Status(status) != expectedStatus {
		t.Fatalf("expected payment status %q, got %q err=%v", expectedStatus, status, err)
	}
	var payload []byte
	if err := pool.QueryRow(ctx, `SELECT payload FROM outbox_events WHERE subject = $1`, expectedSubject).Scan(&payload); err != nil {
		t.Fatalf("read payment result outbox event: %v", err)
	}
	switch expectedStatus {
	case StatusSucceeded:
		var event paymentsv1.PaymentSucceeded
		if err := proto.Unmarshal(payload, &event); err != nil || event.GetEventId() == "" || event.GetOccurredAt() == nil || event.GetPaymentId() != payment.ID || event.GetOrderId() != payment.OrderID {
			t.Fatalf("unexpected payment-succeeded event: event=%+v err=%v", &event, err)
		}
	case StatusFailed:
		var event paymentsv1.PaymentFailed
		if err := proto.Unmarshal(payload, &event); err != nil || event.GetEventId() == "" || event.GetOccurredAt() == nil || event.GetPaymentId() != payment.ID || event.GetOrderId() != payment.OrderID {
			t.Fatalf("unexpected payment-failed event: event=%+v err=%v", &event, err)
		}
	default:
		t.Fatalf("unsupported expected payment status %q", expectedStatus)
	}
}
