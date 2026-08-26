package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"polyglot-ticketing-v1/apps/tickets/internal/projection"
	"polyglot-ticketing-v1/apps/tickets/internal/ticket"
)

const (
	orderDurableName       = "tickets-order-reservation-v1"
	orderStreamName        = "ORDERS_EVENTS"
	orderConnectTimeout    = 5 * time.Second
	orderReconnectInterval = time.Second
)

type OrderConsumer struct {
	handler *projection.OrderHandler
	logger  *slog.Logger
	url     string
}

func NewOrderConsumer(repository ticket.ReservationRepository, url string, logger *slog.Logger) *OrderConsumer {
	return &OrderConsumer{
		handler: projection.NewOrderHandler(repository),
		logger:  logger,
		url:     url,
	}
}

func (consumer *OrderConsumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := consumer.consume(ctx); err != nil && !errors.Is(err, context.Canceled) {
			consumer.logger.Warn("order reservation consumer stopped", "error", err)
		}
		if !waitForOrderRetry(ctx) {
			return
		}
	}
}

func (consumer *OrderConsumer) consume(ctx context.Context) error {
	nc, err := nats.Connect(consumer.url, nats.Timeout(orderConnectTimeout))
	if err != nil {
		return err
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		return err
	}
	subscription, err := js.PullSubscribe(
		"orders.order.created.v1",
		orderDurableName,
		nats.BindStream(orderStreamName),
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

func (consumer *OrderConsumer) handleMessage(ctx context.Context, message *nats.Msg) {
	err := consumer.handler.Handle(ctx, message.Subject, message.Data)
	if err == nil {
		if err := message.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge order event", "error", err)
		}
		return
	}
	if errors.Is(err, ticket.ErrInvalidOrderEvent) || errors.Is(err, ticket.ErrUnsupportedOrderEvent) {
		consumer.logger.Error("terminal order event", "subject", message.Subject, "error", err)
		if termErr := message.Term(); termErr != nil {
			consumer.logger.Warn("failed to terminate order event", "error", termErr)
		}
		return
	}

	consumer.logger.Warn("ticket reservation failed; event will be retried", "subject", message.Subject, "error", err)
	if nakErr := message.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge order event", "error", nakErr)
	}
}

func waitForOrderRetry(ctx context.Context) bool {
	timer := time.NewTimer(orderReconnectInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
