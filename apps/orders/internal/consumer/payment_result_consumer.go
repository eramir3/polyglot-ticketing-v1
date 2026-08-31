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
	paymentFailedDurableName    = "orders-payment-failed-v1"
	paymentSucceededDurableName = "orders-payment-succeeded-v1"
)

type PaymentResultConsumer struct {
	deadLetters ordersDeadLetterer
	durable     string
	handler     *order.PaymentResultHandler
	logger      *slog.Logger
	metrics     *observability.Metrics
	operation   string
	subject     string
	url         string
}

func NewPaymentFailedConsumer(repository order.PaymentResultEventRepository, url string, logger *slog.Logger, metrics ...*observability.Metrics) *PaymentResultConsumer {
	return newPaymentResultConsumer(repository, paymentevents.PaymentFailedSubject, paymentFailedDurableName, "payment_failed", url, logger, firstMetrics(metrics))
}

func NewPaymentSucceededConsumer(repository order.PaymentResultEventRepository, url string, logger *slog.Logger, metrics ...*observability.Metrics) *PaymentResultConsumer {
	return newPaymentResultConsumer(repository, paymentevents.PaymentSucceededSubject, paymentSucceededDurableName, "payment_succeeded", url, logger, firstMetrics(metrics))
}

func newPaymentResultConsumer(
	repository order.PaymentResultEventRepository,
	subject string,
	durable string,
	operation string,
	url string,
	logger *slog.Logger,
	metrics *observability.Metrics,
) *PaymentResultConsumer {
	return &PaymentResultConsumer{
		durable:   durable,
		handler:   order.NewPaymentResultHandler(repository),
		logger:    logger,
		metrics:   metrics,
		operation: operation,
		subject:   subject,
		url:       url,
	}
}

func (consumer *PaymentResultConsumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := consumer.consume(ctx); err != nil && !errors.Is(err, context.Canceled) {
			consumer.logger.Warn("payment-result consumer stopped", "subject", consumer.subject, "error", err)
		}
		if !waitForRetry(ctx) {
			return
		}
	}
}

func (consumer *PaymentResultConsumer) consume(ctx context.Context) error {
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
		consumer.subject,
		consumer.durable,
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

func (consumer *PaymentResultConsumer) handleMessage(ctx context.Context, message *nats.Msg) {
	ctx = tracing.ExtractNATS(ctx, message.Header)
	event, err := newOrdersDelivery(message, consumer.deadLetterSubject())
	if err != nil {
		consumer.logger.Warn("failed to read payment result delivery metadata; event will be retried", "subject", consumer.subject, "error", err)
		if nakErr := message.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge payment result event", "subject", consumer.subject, "error", nakErr)
		}
		return
	}
	consumer.handleDelivery(ctx, event)
}

func (consumer *PaymentResultConsumer) handleDelivery(ctx context.Context, event ordersDelivery) {
	started := time.Now()
	err := consumer.handler.Handle(ctx, consumer.subject, event.payload)
	if err == nil {
		if err := event.delivery.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge payment result event", "subject", consumer.subject, "error", err)
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
	consumer.logger.Warn("payment result handling failed; event will be retried", "subject", consumer.subject, "error", err)
	if nakErr := event.delivery.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge payment result event", "subject", consumer.subject, "error", nakErr)
	}
	consumer.observe("retry", started)
}

func (consumer *PaymentResultConsumer) deadLetterSubject() string {
	if consumer.subject == paymentevents.PaymentSucceededSubject {
		return orderevents.PaymentSucceededDeadLetterSubject
	}
	return orderevents.PaymentFailedDeadLetterSubject
}

func (consumer *PaymentResultConsumer) park(ctx context.Context, event ordersDelivery, failureClass string, failure error) bool {
	if consumer.deadLetters == nil {
		consumer.logger.Warn("payment result dead letter queue is unavailable; event will be retried", "subject", consumer.subject, "error", failure)
		if nakErr := event.delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge payment result event", "subject", consumer.subject, "error", nakErr)
		}
		return false
	}
	if err := consumer.deadLetters.Park(ctx, event.deadLetter(consumer.durable, failureClass, failure)); err != nil {
		consumer.logger.Error("failed to park payment result event in dead letter queue; event will be retried", "subject", consumer.subject, "error", err)
		if nakErr := event.delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge payment result event", "subject", consumer.subject, "error", nakErr)
		}
		return false
	}
	if err := event.delivery.Ack(); err != nil {
		consumer.logger.Warn("failed to acknowledge payment result event after dead-lettering", "subject", consumer.subject, "error", err)
		return false
	}
	consumer.logger.Warn("payment result event parked in dead letter queue", "subject", consumer.subject, "stream_sequence", event.streamSeq, "delivery_count", event.deliveryCount, "failure_class", failureClass, "error", failure)
	return true
}

func (consumer *PaymentResultConsumer) observe(outcome string, started time.Time) {
	if consumer.metrics != nil {
		consumer.metrics.ObserveBackground("jetstream_consumer", consumer.operation, outcome, started)
	}
}
