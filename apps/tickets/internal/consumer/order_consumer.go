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
	ticketevents "polyglot-ticketing-v1/contracts/tickets"
	"polyglot-ticketing-v1/internal/observability"
	"polyglot-ticketing-v1/internal/tracing"
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
	deadLetters ticketsDeadLetterer
	durableName string
	handler     *projection.OrderHandler
	logger      *slog.Logger
	metrics     *observability.Metrics
	operation   string
	subject     string
	url         string
}

type orderEventDelivery interface {
	Ack(...nats.AckOpt) error
	Nak(...nats.AckOpt) error
	NakWithDelay(time.Duration, ...nats.AckOpt) error
	Term(...nats.AckOpt) error
}

func NewOrderCreatedConsumer(repository ticket.ReservationRepository, url string, logger *slog.Logger, metrics ...*observability.Metrics) *OrderConsumer {
	return newOrderConsumer(repository, orderCreatedDurableName, orderevents.OrderCreatedSubject, "order_created", url, logger, firstMetrics(metrics))
}

func NewOrderCanceledConsumer(repository ticket.ReservationRepository, url string, logger *slog.Logger, metrics ...*observability.Metrics) *OrderConsumer {
	return newOrderConsumer(repository, orderCanceledDurableName, orderevents.OrderCanceledSubject, "order_canceled", url, logger, firstMetrics(metrics))
}

func newOrderConsumer(
	repository ticket.ReservationRepository,
	durableName string,
	subject string,
	operation string,
	url string,
	logger *slog.Logger,
	metrics *observability.Metrics,
) *OrderConsumer {
	return &OrderConsumer{
		durableName: durableName,
		handler:     projection.NewOrderHandler(repository),
		logger:      logger,
		metrics:     metrics,
		operation:   operation,
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
	if err := ensureTicketsDeadLetterStream(js); err != nil {
		return err
	}
	consumer.deadLetters = jetStreamTicketsDeadLetterer{js: js}
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
	ctx = tracing.ExtractNATS(ctx, message.Header)
	event, err := newTicketsDelivery(message, consumer.deadLetterSubject())
	if err != nil {
		consumer.logger.Warn("failed to read order event delivery metadata; event will be retried", "error", err)
		if nakErr := message.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge order event", "error", nakErr)
		}
		return
	}
	consumer.handleDelivery(ctx, event)
}

func (consumer *OrderConsumer) handleDelivery(ctx context.Context, event ticketsDelivery) {
	started := time.Now()
	err := consumer.handler.Handle(ctx, event.subject, event.payload)
	if err == nil {
		if err := event.delivery.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge order event", "error", err)
			consumer.observe("ack_failed", started)
			return
		}
		consumer.observe("success", started)
		return
	}
	failureClass := orderFailureClass(err)
	if failureClass == "invalid" || event.deliveryCount > orderEventMaxRetries {
		if consumer.park(ctx, event, failureClass, err) {
			consumer.observe("dead_lettered", started)
			return
		}
		consumer.observe("dead_letter_publish_failed", started)
		return
	}
	if errors.Is(err, ticket.ErrOrderReservationPending) {
		consumer.logger.Warn("ticket order reservation is not available yet; event will be retried", "subject", event.subject, "error", err)
		if nakErr := event.delivery.NakWithDelay(orderReservationRetryDelay); nakErr != nil {
			consumer.logger.Warn("failed to delay order event retry", "error", nakErr)
		}
		consumer.observe("retry", started)
		return
	}

	consumer.logger.Warn("ticket order event failed; event will be retried", "subject", event.subject, "error", err)
	if nakErr := event.delivery.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge order event", "error", nakErr)
	}
	consumer.observe("retry", started)
}

func (consumer *OrderConsumer) deadLetterSubject() string {
	if consumer.subject == orderevents.OrderCanceledSubject {
		return ticketevents.OrderCancellationDeadLetterSubject
	}
	return ticketevents.OrderReservationDeadLetterSubject
}

func (consumer *OrderConsumer) park(ctx context.Context, event ticketsDelivery, failureClass string, failure error) bool {
	if consumer.deadLetters == nil {
		consumer.logger.Warn("order event dead letter queue is unavailable; event will be retried", "subject", event.subject, "error", failure)
		if nakErr := event.delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge order event", "error", nakErr)
		}
		return false
	}

	if err := consumer.deadLetters.Park(ctx, event.deadLetter(consumer.durableName, failureClass, failure)); err != nil {
		consumer.logger.Error("failed to park order event in dead letter queue; event will be retried", "subject", event.subject, "error", err)
		if nakErr := event.delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge order event", "error", nakErr)
		}
		return false
	}
	if err := event.delivery.Ack(); err != nil {
		consumer.logger.Warn("failed to acknowledge order event after dead-lettering", "error", err)
		return false
	}
	consumer.logger.Warn("order event parked in dead letter queue", "subject", event.subject, "stream_sequence", event.streamSeq, "delivery_count", event.deliveryCount, "failure_class", failureClass, "error", failure)
	return true
}

func orderFailureClass(err error) string {
	if errors.Is(err, ticket.ErrInvalidOrderEvent) || errors.Is(err, ticket.ErrUnsupportedOrderEvent) {
		return "invalid"
	}
	if errors.Is(err, ticket.ErrOrderReservationPending) {
		return "reservation_pending"
	}
	return "retryable"
}

func (consumer *OrderConsumer) observe(outcome string, started time.Time) {
	if consumer.metrics != nil {
		consumer.metrics.ObserveBackground("jetstream_consumer", consumer.operation, outcome, started)
	}
}

func firstMetrics(metrics []*observability.Metrics) *observability.Metrics {
	if len(metrics) == 0 {
		return nil
	}
	return metrics[0]
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
