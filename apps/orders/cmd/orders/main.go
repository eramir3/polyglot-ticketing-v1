package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"polyglot-ticketing-v1/apps/orders/internal/consumer"
	"polyglot-ticketing-v1/apps/orders/internal/order"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, requiredEnvironmentVariable("DATABASE_URL"))
	if err != nil {
		slog.Error("failed to create orders database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	consumer.NewTicketConsumer(
		order.NewPostgresRepository(pool),
		environmentVariable("NATS_URL", "nats://localhost:4222"),
		slog.Default(),
	).Run(ctx)
}

func environmentVariable(name string, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

func requiredEnvironmentVariable(name string) string {
	value := os.Getenv(name)
	if value == "" {
		slog.Error("required environment variable is missing", "name", name)
		os.Exit(1)
	}

	return value
}
