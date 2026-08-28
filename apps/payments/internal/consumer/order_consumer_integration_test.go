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
	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/payments/internal/payment"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

func TestOrderConsumerPersistsCreatedOrderProjection(t *testing.T) {
	ctx := context.Background()
	pool, js := startOrderProjectionConsumer(t, ctx)

	eventID := uuid.NewString()
	orderID := uuid.NewString()
	payload, err := proto.Marshal(&ordersv1.OrderCreated{
		EventId:          eventID,
		OccurredAt:       timestamppb.New(time.Now().UTC()),
		OrderId:          orderID,
		OrderStatus:      ordersv1.OrderStatus_ORDER_STATUS_CREATED,
		UserId:           "user-1",
		ExpiresAt:        timestamppb.New(time.Now().UTC().Add(15 * time.Minute)),
		AggregateVersion: 0,
		Ticket:           &ordersv1.OrderTicket{Id: uuid.NewString(), Price: 10_000},
	})
	if err != nil {
		t.Fatalf("marshal order-created event: %v", err)
	}
	if _, err := js.Publish(orderevents.OrderCreatedSubject, payload); err != nil {
		t.Fatalf("publish order-created event: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var storedOrder payment.Order
		projectionErr := pool.QueryRow(ctx, `
			SELECT id::text, aggregate_version, user_id, price, status::text
			FROM orders
			WHERE id = $1`, orderID,
		).Scan(
			&storedOrder.ID,
			&storedOrder.AggregateVersion,
			&storedOrder.UserID,
			&storedOrder.Price,
			&storedOrder.Status,
		)
		var processedCount int
		processedErr := pool.QueryRow(ctx, `SELECT COUNT(*) FROM processed_events WHERE event_id = $1`, eventID).Scan(&processedCount)
		info, consumerErr := js.ConsumerInfo(orderEventsStreamName, orderConsumerDurableName)
		if projectionErr == nil && processedErr == nil && consumerErr == nil &&
			storedOrder == (payment.Order{
				ID:               orderID,
				AggregateVersion: 0,
				UserID:           "user-1",
				Price:            10_000,
				Status:           payment.OrderStatusCreated,
			}) &&
			processedCount == 1 && info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 &&
			info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("order-created event was not persisted and acknowledged")
}

func TestOrderConsumerUpdatesProjectedOrderWhenCanceled(t *testing.T) {
	ctx := context.Background()
	pool, js := startOrderProjectionConsumer(t, ctx)
	orderID := uuid.NewString()
	if _, err := pool.Exec(ctx, `
		INSERT INTO orders (id, aggregate_version, user_id, price, status)
		VALUES ($1, $2, $3, $4, $5)`, orderID, 0, "user-1", 10_000, payment.OrderStatusCreated); err != nil {
		t.Fatalf("seed created order projection: %v", err)
	}

	eventID := uuid.NewString()
	payload, err := proto.Marshal(&ordersv1.OrderCanceled{
		EventId:          eventID,
		OccurredAt:       timestamppb.New(time.Now().UTC()),
		OrderId:          orderID,
		AggregateVersion: 1,
		Ticket:           &ordersv1.OrderCanceledTicket{Id: uuid.NewString()},
	})
	if err != nil {
		t.Fatalf("marshal order-canceled event: %v", err)
	}
	if _, err := js.Publish(orderevents.OrderCanceledSubject, payload); err != nil {
		t.Fatalf("publish order-canceled event: %v", err)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var status string
		var aggregateVersion int64
		projectionErr := pool.QueryRow(ctx, `
			SELECT status::text, aggregate_version
			FROM orders
			WHERE id = $1`, orderID,
		).Scan(&status, &aggregateVersion)
		var processedCount int
		processedErr := pool.QueryRow(ctx, `SELECT COUNT(*) FROM processed_events WHERE event_id = $1`, eventID).Scan(&processedCount)
		info, consumerErr := js.ConsumerInfo(orderEventsStreamName, orderConsumerDurableName)
		if projectionErr == nil && processedErr == nil && consumerErr == nil &&
			status == string(payment.OrderStatusCanceled) && aggregateVersion == 1 && processedCount == 1 &&
			info.Delivered.Stream == 1 && info.AckFloor.Stream == 1 &&
			info.NumAckPending == 0 && info.NumPending == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("order-canceled event did not update and acknowledge the projected order")
}

func startOrderProjectionConsumer(t *testing.T, ctx context.Context) (*pgxpool.Pool, nats.JetStreamContext) {
	t.Helper()
	pool := startPaymentsPostgres(t, ctx)
	nc, js, shutdown := startOrderJetStream(t)
	t.Cleanup(shutdown)
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:     orderEventsStreamName,
		Subjects: []string{"orders.>"},
	}); err != nil {
		t.Fatalf("add orders events stream: %v", err)
	}

	consumeCtx, cancel := context.WithCancel(context.Background())
	consumerDone := make(chan struct{})
	go func() {
		NewOrderConsumer(payment.NewPostgresRepository(pool), nc.ConnectedUrl(), testLogger()).Run(consumeCtx)
		close(consumerDone)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-consumerDone:
		case <-time.After(2 * time.Second):
			t.Error("order consumer did not stop")
		}
	})
	return pool, js
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

func startOrderJetStream(t *testing.T) (*nats.Conn, nats.JetStreamContext, func()) {
	t.Helper()
	server, err := natsserver.NewServer(&natsserver.Options{
		JetStream: true,
		NoLog:     true,
		NoSigs:    true,
		Port:      -1,
		StoreDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create JetStream server: %v", err)
	}
	go server.Start()
	if !server.ReadyForConnections(2 * time.Second) {
		server.Shutdown()
		t.Fatal("JetStream server did not become ready")
	}

	nc, err := nats.Connect(server.ClientURL(), nats.Timeout(time.Second))
	if err != nil {
		server.Shutdown()
		t.Fatalf("connect to JetStream server: %v", err)
	}
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		server.Shutdown()
		t.Fatalf("create JetStream context: %v", err)
	}
	return nc, js, func() {
		nc.Close()
		server.Shutdown()
	}
}
