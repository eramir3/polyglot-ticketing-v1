package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/nats-io/nats.go"

	"polyglot-ticketing-v1/apps/payments/internal/consumer"
)

func main() {
	sequence, err := requiredSequence("DLQ_SEQUENCE")
	if err != nil {
		slog.Error("invalid DLQ replay request", "error", err)
		os.Exit(1)
	}

	natsURL := os.Getenv("NATS_URL")
	if natsURL == "" {
		natsURL = "nats://localhost:4222"
	}
	nc, err := nats.Connect(natsURL, nats.Timeout(5*time.Second))
	if err != nil {
		slog.Error("failed to connect to NATS", "error", err)
		os.Exit(1)
	}
	defer nc.Close()
	js, err := nc.JetStream()
	if err != nil {
		slog.Error("failed to create JetStream context", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ack, err := consumer.ReplayPaymentsDeadLetter(ctx, js, sequence)
	if err != nil {
		slog.Error("Payments DLQ replay failed", "sequence", sequence, "error", err)
		os.Exit(1)
	}
	fmt.Printf("replayed Payments DLQ sequence %d as source stream sequence %d\n", sequence, ack.Sequence)
}

func requiredSequence(name string) (uint64, error) {
	value := os.Getenv(name)
	if value == "" {
		return 0, fmt.Errorf("%s is required", name)
	}
	sequence, err := strconv.ParseUint(value, 10, 64)
	if err != nil || sequence == 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return sequence, nil
}
