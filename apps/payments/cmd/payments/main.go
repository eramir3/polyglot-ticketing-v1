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

	"polyglot-ticketing-v1/apps/payments/internal/consumer"
	grpcserver "polyglot-ticketing-v1/apps/payments/internal/grpc"
	"polyglot-ticketing-v1/apps/payments/internal/payment"
	"polyglot-ticketing-v1/internal/outbox"
	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, requiredEnvironmentVariable("DATABASE_URL"))
	if err != nil {
		slog.Error("failed to create payments database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	repository := payment.NewPostgresRepository(pool)
	natsURL := environmentVariable("NATS_URL", "nats://localhost:4222")
	go outbox.NewPublisher(
		outbox.NewPostgresRepository(pool),
		outbox.Config{StreamName: "PAYMENTS_EVENTS", Subjects: []string{"payments.>"}},
		natsURL,
		slog.Default(),
	).Run(ctx)
	go consumer.NewOrderConsumer(repository, natsURL, slog.Default()).Run(ctx)

	listener, err := net.Listen("tcp", ":"+environmentVariable("GRPC_PORT", "50054"))
	if err != nil {
		slog.Error("failed to listen for gRPC", "error", err)
		os.Exit(1)
	}

	server := grpc.NewServer()
	paymentsv1.RegisterPaymentsServiceServer(server, grpcserver.NewServer(payment.NewService(repository)))
	go func() {
		slog.Info("payments gRPC service started", "address", listener.Addr().String())
		if err := server.Serve(listener); err != nil {
			slog.Error("payments gRPC service stopped unexpectedly", "error", err)
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
