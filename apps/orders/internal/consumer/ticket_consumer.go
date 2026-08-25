package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	"polyglot-ticketing-v1/apps/orders/internal/projection"
)

const (
	durableName       = "orders-ticket-projection-v1"
	streamName        = "TICKETS_EVENTS"
	connectTimeout    = 5 * time.Second
	reconnectInterval = time.Second
)

type TicketConsumer struct {
	handler *projection.TicketHandler
	logger  *slog.Logger
	url     string
}

func NewTicketConsumer(repository order.TicketProjectionRepository, url string, logger *slog.Logger) *TicketConsumer {
	return &TicketConsumer{
		handler: projection.NewTicketHandler(repository),
		logger:  logger,
		url:     url,
	}
}

func (consumer *TicketConsumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := consumer.consume(ctx); err != nil && !errors.Is(err, context.Canceled) {
			consumer.logger.Warn("ticket projection consumer stopped", "error", err)
		}
		if !waitForRetry(ctx) {
			return
		}
	}
}

func (consumer *TicketConsumer) consume(ctx context.Context) error {
	nc, err := nats.Connect(consumer.url, nats.Timeout(connectTimeout))
	if err != nil {
		return err
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		return err
	}
	subscription, err := js.PullSubscribe(
		"tickets.>",
		durableName,
		nats.BindStream(streamName),
		nats.DeliverAll(),
		nats.ManualAck(),
	)
	if err != nil {
		return err
	}

	for ctx.Err() == nil {
		messages, err := subscription.Fetch(1, nats.MaxWait(time.Second))
		if errors.Is(err, nats.ErrTimeout) {
			continue
		}
		if err != nil {
			return err
		}
		for _, message := range messages {
			consumer.handleMessage(ctx, message)
		}
	}
	return context.Canceled
}

func (consumer *TicketConsumer) handleMessage(ctx context.Context, message *nats.Msg) {
	err := consumer.handler.Handle(ctx, message.Subject, message.Data)
	if err == nil {
		if err := message.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge ticket event", "error", err)
		}
		return
	}
	if errors.Is(err, order.ErrInvalidTicketEvent) || errors.Is(err, order.ErrUnsupportedSubject) {
		consumer.logger.Error("terminal ticket event", "subject", message.Subject, "error", err)
		if termErr := message.Term(); termErr != nil {
			consumer.logger.Warn("failed to terminate ticket event", "error", termErr)
		}
		return
	}

	consumer.logger.Warn("ticket projection failed; event will be retried", "subject", message.Subject, "error", err)
	if nakErr := message.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge ticket event", "error", nakErr)
	}
}

func waitForRetry(ctx context.Context) bool {
	timer := time.NewTimer(reconnectInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
