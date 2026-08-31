package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	paymentevents "polyglot-ticketing-v1/contracts/payments"
	"polyglot-ticketing-v1/internal/observability"
	"polyglot-ticketing-v1/internal/tracing"
)

const (
	paymentCreatedDurableName = "orders-payment-created-v1"
	paymentEventsStreamName   = "PAYMENTS_EVENTS"
)

type PaymentCreatedConsumer struct {
	deadLetters ordersDeadLetterer
	handler     *order.PaymentCreatedHandler
	logger      *slog.Logger
	metrics     *observability.Metrics
	url         string
}

func NewPaymentCreatedConsumer(
	repository order.PaymentEventRepository,
	url string,
	logger *slog.Logger,
	metrics ...*observability.Metrics,
) *PaymentCreatedConsumer {
	return &PaymentCreatedConsumer{
		handler: order.NewPaymentCreatedHandler(repository),
		logger:  logger,
		metrics: firstMetrics(metrics),
		url:     url,
	}
}

func (consumer *PaymentCreatedConsumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := consumer.consume(ctx); err != nil && !errors.Is(err, context.Canceled) {
			consumer.logger.Warn("payment-created consumer stopped", "error", err)
		}
		if !waitForRetry(ctx) {
			return
		}
	}
}

func (consumer *PaymentCreatedConsumer) consume(ctx context.Context) error {
	nc, err := nats.Connect(consumer.url, nats.Timeout(connectTimeout))
	if err != nil {
		return err
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		return err
	}
	if err := ensureOrdersDeadLetterStream(js); err != nil {
		return err
	}
	consumer.deadLetters = jetStreamOrdersDeadLetterer{js: js}
	subscription, err := js.PullSubscribe(
		paymentevents.PaymentCreatedSubject,
		paymentCreatedDurableName,
		nats.BindStream(paymentEventsStreamName),
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

func (consumer *PaymentCreatedConsumer) handleMessage(ctx context.Context, message *nats.Msg) {
	ctx = tracing.ExtractNATS(ctx, message.Header)
	event, err := newOrdersDelivery(message, orderevents.PaymentCreatedDeadLetterSubject)
	if err != nil {
		consumer.logger.Warn("failed to read payment-created delivery metadata; event will be retried", "error", err)
		if nakErr := message.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge payment-created event", "error", nakErr)
		}
		return
	}
	consumer.handleDelivery(ctx, event)
}

func (consumer *PaymentCreatedConsumer) handleDelivery(ctx context.Context, event ordersDelivery) {
	started := time.Now()
	err := consumer.handler.Handle(ctx, event.payload)
	if err == nil {
		if err := event.delivery.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge payment-created event", "error", err)
			consumer.observe("ack_failed", started)
			return
		}
		consumer.observe("success", started)
		return
	}
	if errors.Is(err, order.ErrInvalidPaymentEvent) || event.deliveryCount > orderEventMaxRetries {
		if consumer.park(ctx, event, paymentFailureClass(err), err) {
			consumer.observe("dead_lettered", started)
		} else {
			consumer.observe("dead_letter_publish_failed", started)
		}
		return
	}

	consumer.logger.Warn("payment-created handling failed; event will be retried", "error", err)
	if nakErr := event.delivery.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge payment-created event", "error", nakErr)
	}
	consumer.observe("retry", started)
}

func (consumer *PaymentCreatedConsumer) park(ctx context.Context, event ordersDelivery, failureClass string, failure error) bool {
	if consumer.deadLetters == nil {
		consumer.logger.Warn("payment-created dead letter queue is unavailable; event will be retried", "subject", event.subject, "error", failure)
		if nakErr := event.delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge payment-created event", "error", nakErr)
		}
		return false
	}
	if err := consumer.deadLetters.Park(ctx, event.deadLetter(paymentCreatedDurableName, failureClass, failure)); err != nil {
		consumer.logger.Error("failed to park payment-created event in dead letter queue; event will be retried", "subject", event.subject, "error", err)
		if nakErr := event.delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge payment-created event", "error", nakErr)
		}
		return false
	}
	if err := event.delivery.Ack(); err != nil {
		consumer.logger.Warn("failed to acknowledge payment-created event after dead-lettering", "error", err)
		return false
	}
	consumer.logger.Warn("payment-created event parked in dead letter queue", "subject", event.subject, "stream_sequence", event.streamSeq, "delivery_count", event.deliveryCount, "failure_class", failureClass, "error", failure)
	return true
}

func paymentFailureClass(err error) string {
	if errors.Is(err, order.ErrInvalidPaymentEvent) {
		return "invalid"
	}
	return "retryable"
}

func (consumer *PaymentCreatedConsumer) observe(outcome string, started time.Time) {
	if consumer.metrics != nil {
		consumer.metrics.ObserveBackground("jetstream_consumer", "payment_created", outcome, started)
	}
}
