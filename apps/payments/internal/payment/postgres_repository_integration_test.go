package payment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
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

	existing, wasCreated, err := repository.Create(ctx, CreateInput{OrderID: orderID, UserID: "user-1"})
	if err != nil || wasCreated || existing != created {
		t.Fatalf("repeat payment: payment=%+v created=%t err=%v", existing, wasCreated, err)
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
