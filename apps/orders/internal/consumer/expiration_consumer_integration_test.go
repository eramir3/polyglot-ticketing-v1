package consumer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	expirationevents "polyglot-ticketing-v1/contracts/expiration"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	expirationv1 "polyglot-ticketing-v1/protogen/go/expiration/v1"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

func TestExpirationCompleteConsumerIntegration(t *testing.T) {
	ctx := context.Background()
	pool := startOrdersPostgres(t, ctx)
	orderID, ticketID := seedCreatedOrder(t, ctx, pool)

	nc, js, shutdown := startTicketJetStream(t)
	defer shutdown()
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     expirationEventsStreamName,
		Subjects: []string{"expiration.>"},
	}); err != nil {
		t.Fatalf("add expiration events stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	consumer := NewExpirationCompleteConsumer(order.NewPostgresRepository(pool), nc.ConnectedUrl(), testLogger())
	go func() {
		consumer.Run(consumeCtx)
		close(consumerDone)
	}()
	defer func() {
		cancel()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
			t.Error("expiration-complete consumer did not stop")
		}
	}()

	eventID := uuid.NewString()
	payload, err := proto.Marshal(&expirationv1.ExpirationComplete{
		EventId:    eventID,
		OccurredAt: timestamppb.New(time.Now().UTC()),
		OrderId:    orderID,
	})
	if err != nil {
		t.Fatalf("marshal expiration-complete event: %v", err)
	}
	if _, err := js.Publish(expirationevents.ExpirationCompleteSubject, payload); err != nil {
		t.Fatalf("publish expiration-complete event: %v", err)
	}

	waitForExpirationCompleteProcessing(t, ctx, pool, js, orderID)

	t.Run("updates the order status to canceled", func(t *testing.T) {
		var status string
		if err := pool.QueryRow(ctx, `SELECT status::text FROM orders WHERE id = $1`, orderID).Scan(&status); err != nil {
			t.Fatalf("read order status: %v", err)
		}
		if status != string(order.StatusCanceled) {
			t.Fatalf("expected canceled order status, got %q", status)
		}
	})

	t.Run("emit an OrderCanceled event", func(t *testing.T) {
		var storedEventID string
		var storedPayload []byte
		if err := pool.QueryRow(ctx, `
			SELECT event_id::text, payload
			FROM outbox_events
			WHERE subject = $1`, orderevents.OrderCanceledSubject,
		).Scan(&storedEventID, &storedPayload); err != nil {
			t.Fatalf("read OrderCanceled outbox event: %v", err)
		}

		var event ordersv1.OrderCanceled
		if err := proto.Unmarshal(storedPayload, &event); err != nil {
			t.Fatalf("unmarshal OrderCanceled outbox event: %v", err)
		}
		if event.GetEventId() != storedEventID || event.GetOrderId() != orderID ||
			event.GetTicket().GetId() != ticketID || event.GetAggregateVersion() != 1 {
			t.Fatalf("unexpected OrderCanceled event: %+v", &event)
		}
	})

	t.Run("ack the message", func(t *testing.T) {
		info, err := js.ConsumerInfo(expirationEventsStreamName, expirationCompleteDurableName)
		if err != nil {
			t.Fatalf("read expiration-complete consumer info: %v", err)
		}
		if info.Delivered.Stream != 1 || info.AckFloor.Stream != 1 ||
			info.NumAckPending != 0 || info.NumPending != 0 {
			t.Fatalf("expiration-complete event was not acknowledged: %+v", info)
		}
	})
}

func startOrdersPostgres(t *testing.T, ctx context.Context) *pgxpool.Pool {
	t.Helper()
	container, err := postgres.Run(
		ctx,
		"postgres:17-alpine",
		postgres.BasicWaitStrategies(),
		postgres.WithDatabase("orders"),
		postgres.WithUsername("orders"),
		postgres.WithPassword("orders-test-password"),
	)
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("start orders PostgreSQL container: %v", err)
	}

	databaseURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("get orders PostgreSQL connection string: %v", err)
	}
	applyOrdersMigrations(t, databaseURL)

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("create orders database pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func applyOrdersMigrations(t *testing.T, databaseURL string) {
	t.Helper()
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get test working directory: %v", err)
	}
	migrationsPath := filepath.Join(workingDirectory, "..", "..", "migrations")
	migrator, err := migrate.New(fmt.Sprintf("file://%s", migrationsPath), databaseURL)
	if err != nil {
		t.Fatalf("initialize orders migrations: %v", err)
	}
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})
	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("apply orders migrations: %v", err)
	}
}

func seedCreatedOrder(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	ticketID := uuid.NewString()
	orderID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO tickets (id, title, price, aggregate_version)
		VALUES ($1, $2, $3, $4)`, ticketID, "Projected concert ticket", 10_000, 0); err != nil {
		t.Fatalf("seed ticket projection: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO orders (id, expires_at, user_id, ticket_id, status, aggregate_version)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		orderID,
		time.Now().UTC().Add(time.Minute),
		"user-1",
		ticketID,
		order.StatusCreated,
		0,
	); err != nil {
		t.Fatalf("seed created order: %v", err)
	}
	return orderID, ticketID
}

func waitForExpirationCompleteProcessing(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	js nats.JetStreamContext,
	orderID string,
) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		statusErr := pool.QueryRow(ctx, `SELECT status::text FROM orders WHERE id = $1`, orderID).Scan(&status)
		var eventCount int
		eventErr := pool.QueryRow(ctx, `
			SELECT COUNT(*)
			FROM outbox_events
			WHERE subject = $1`, orderevents.OrderCanceledSubject).Scan(&eventCount)
		info, consumerErr := js.ConsumerInfo(expirationEventsStreamName, expirationCompleteDurableName)

		if statusErr == nil && eventErr == nil && consumerErr == nil &&
			status == string(order.StatusCanceled) && eventCount == 1 &&
			info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 &&
			info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("expiration-complete event was not fully processed")
}
