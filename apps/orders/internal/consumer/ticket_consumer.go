package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	"polyglot-ticketing-v1/apps/orders/internal/projection"
	"polyglot-ticketing-v1/internal/observability"
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
	metrics *observability.Metrics
	url     string
}

type ticketEventDelivery interface {
	Ack(...nats.AckOpt) error
	Nak(...nats.AckOpt) error
	Term(...nats.AckOpt) error
}

func NewTicketConsumer(repository order.TicketProjectionRepository, url string, logger *slog.Logger, metrics ...*observability.Metrics) *TicketConsumer {
	return &TicketConsumer{
		handler: projection.NewTicketHandler(repository),
		logger:  logger,
		metrics: firstMetrics(metrics),
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
	consumer.handleDelivery(ctx, message.Subject, message.Data, message)
}

func (consumer *TicketConsumer) handleDelivery(
	ctx context.Context,
	subject string,
	payload []byte,
	delivery ticketEventDelivery,
) {
	started := time.Now()
	err := consumer.handler.Handle(ctx, subject, payload)
	if err == nil {
		if err := delivery.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge ticket event", "error", err)
			consumer.observe("ack_failed", started)
			return
		}
		consumer.observe("success", started)
		return
	}
	if errors.Is(err, order.ErrInvalidTicketEvent) || errors.Is(err, order.ErrUnsupportedSubject) {
		consumer.logger.Error("terminal ticket event", "subject", subject, "error", err)
		if termErr := delivery.Term(); termErr != nil {
			consumer.logger.Warn("failed to terminate ticket event", "error", termErr)
		}
		consumer.observe("terminal", started)
		return
	}
	if errors.Is(err, order.ErrTicketEventVersionGap) {
		consumer.logger.Warn("ticket projection has a version gap; event will be retried", "subject", subject, "error", err)
		if nakErr := delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge ticket event", "error", nakErr)
		}
		consumer.observe("retry", started)
		return
	}

	consumer.logger.Warn("ticket projection failed; event will be retried", "subject", subject, "error", err)
	if nakErr := delivery.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge ticket event", "error", nakErr)
	}
	consumer.observe("retry", started)
}

func (consumer *TicketConsumer) observe(outcome string, started time.Time) {
	if consumer.metrics != nil {
		consumer.metrics.ObserveBackground("jetstream_consumer", "ticket_projection", outcome, started)
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
