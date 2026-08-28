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

	"polyglot-ticketing-v1/apps/tickets/internal/consumer"
	grpcserver "polyglot-ticketing-v1/apps/tickets/internal/grpc"
	"polyglot-ticketing-v1/apps/tickets/internal/ticket"
	"polyglot-ticketing-v1/internal/observability"
	"polyglot-ticketing-v1/internal/outbox"
	ticketsv1 "polyglot-ticketing-v1/protogen/go/tickets/v1"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, requiredEnvironmentVariable("DATABASE_URL"))
	if err != nil {
		slog.Error("failed to create tickets database pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	repository := ticket.NewPostgresRepository(pool)
	metrics, registry := observability.NewMetrics()
	observability.StartMetricsServer(ctx, environmentVariable("METRICS_PORT", observability.DefaultMetricsPort), registry, slog.Default())
	natsURL := environmentVariable("NATS_URL", "nats://localhost:4222")
	publisher := outbox.NewPublisher(
		outbox.NewPostgresRepository(pool),
		outbox.Config{StreamName: "TICKETS_EVENTS", Subjects: []string{"tickets.>"}},
		natsURL,
		slog.Default(),
		metrics,
	)
	go publisher.Run(ctx)
	go consumer.NewOrderCreatedConsumer(repository, natsURL, slog.Default(), metrics).Run(ctx)
	go consumer.NewOrderCanceledConsumer(repository, natsURL, slog.Default(), metrics).Run(ctx)

	listener, err := net.Listen("tcp", ":"+environmentVariable("GRPC_PORT", "50052"))
	if err != nil {
		slog.Error("failed to listen for gRPC", "error", err)
		os.Exit(1)
	}

	server := grpc.NewServer(grpc.UnaryInterceptor(metrics.UnaryServerInterceptor))
	ticketsv1.RegisterTicketsServiceServer(
		server,
		grpcserver.NewServer(ticket.NewService(repository), slog.Default()),
	)

	go func() {
		slog.Info("tickets gRPC service started", "address", listener.Addr().String())
		if err := server.Serve(listener); err != nil {
			slog.Error("tickets gRPC service stopped unexpectedly", "error", err)
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
