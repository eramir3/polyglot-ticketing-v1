package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"polyglot-ticketing-v1/apps/tickets/internal/projection"
	"polyglot-ticketing-v1/apps/tickets/internal/ticket"
	orderevents "polyglot-ticketing-v1/contracts/orders"
)

const (
	orderCanceledDurableName   = "tickets-order-cancellation-v1"
	orderCreatedDurableName    = "tickets-order-reservation-v1"
	orderStreamName            = "ORDERS_EVENTS"
	orderConnectTimeout        = 5 * time.Second
	orderReconnectInterval     = time.Second
	orderReservationRetryDelay = time.Second
)

type OrderConsumer struct {
	durableName string
	handler     *projection.OrderHandler
	logger      *slog.Logger
	subject     string
	url         string
}

type orderEventDelivery interface {
	Ack(...nats.AckOpt) error
	Nak(...nats.AckOpt) error
	NakWithDelay(time.Duration, ...nats.AckOpt) error
	Term(...nats.AckOpt) error
}

func NewOrderCreatedConsumer(repository ticket.ReservationRepository, url string, logger *slog.Logger) *OrderConsumer {
	return newOrderConsumer(repository, orderCreatedDurableName, orderevents.OrderCreatedSubject, url, logger)
}

func NewOrderCanceledConsumer(repository ticket.ReservationRepository, url string, logger *slog.Logger) *OrderConsumer {
	return newOrderConsumer(repository, orderCanceledDurableName, orderevents.OrderCanceledSubject, url, logger)
}

func newOrderConsumer(
	repository ticket.ReservationRepository,
	durableName string,
	subject string,
	url string,
	logger *slog.Logger,
) *OrderConsumer {
	return &OrderConsumer{
		durableName: durableName,
		handler:     projection.NewOrderHandler(repository),
		logger:      logger,
		subject:     subject,
		url:         url,
	}
}

func (consumer *OrderConsumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := consumer.consume(ctx); err != nil && !errors.Is(err, context.Canceled) {
			consumer.logger.Warn("order consumer stopped", "error", err)
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
		consumer.subject,
		consumer.durableName,
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
	consumer.handleDelivery(ctx, message.Subject, message.Data, message)
}

func (consumer *OrderConsumer) handleDelivery(
	ctx context.Context,
	subject string,
	payload []byte,
	delivery orderEventDelivery,
) {
	err := consumer.handler.Handle(ctx, subject, payload)
	if err == nil {
		if err := delivery.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge order event", "error", err)
		}
		return
	}
	if errors.Is(err, ticket.ErrInvalidOrderEvent) || errors.Is(err, ticket.ErrUnsupportedOrderEvent) {
		consumer.logger.Error("terminal order event", "subject", subject, "error", err)
		if termErr := delivery.Term(); termErr != nil {
			consumer.logger.Warn("failed to terminate order event", "error", termErr)
		}
		return
	}
	if errors.Is(err, ticket.ErrOrderReservationPending) {
		consumer.logger.Warn("ticket order reservation is not available yet; event will be retried", "subject", subject, "error", err)
		if nakErr := delivery.NakWithDelay(orderReservationRetryDelay); nakErr != nil {
			consumer.logger.Warn("failed to delay order event retry", "error", nakErr)
		}
		return
	}

	consumer.logger.Warn("ticket order event failed; event will be retried", "subject", subject, "error", err)
	if nakErr := delivery.Nak(); nakErr != nil {
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
