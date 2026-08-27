package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"

	"polyglot-ticketing-v1/apps/orders/internal/consumer"
	grpcserver "polyglot-ticketing-v1/apps/orders/internal/grpc"
	"polyglot-ticketing-v1/apps/orders/internal/order"
	"polyglot-ticketing-v1/internal/outbox"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
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

	repository := order.NewPostgresRepository(pool)
	natsURL := environmentVariable("NATS_URL", "nats://localhost:4222")
	go outbox.NewPublisher(
		outbox.NewPostgresRepository(pool),
		outbox.Config{StreamName: "ORDERS_EVENTS", Subjects: []string{"orders.>"}},
		natsURL,
		slog.Default(),
	).Run(ctx)
	go consumer.NewTicketConsumer(
		repository,
		natsURL,
		slog.Default(),
	).Run(ctx)
	go consumer.NewExpirationCompleteConsumer(
		repository,
		natsURL,
		slog.Default(),
	).Run(ctx)

	listener, err := net.Listen("tcp", ":"+environmentVariable("GRPC_PORT", "50053"))
	if err != nil {
		slog.Error("failed to listen for gRPC", "error", err)
		os.Exit(1)
	}

	server := grpc.NewServer()
	ordersv1.RegisterOrdersServiceServer(
		server,
		grpcserver.NewServer(order.NewService(repository)),
	)

	go func() {
		slog.Info("orders gRPC service started", "address", listener.Addr().String())
		if err := server.Serve(listener); err != nil {
			slog.Error("orders gRPC service stopped unexpectedly", "error", err)
		}
	}()

	<-ctx.Done()
	server.GracefulStop()
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
