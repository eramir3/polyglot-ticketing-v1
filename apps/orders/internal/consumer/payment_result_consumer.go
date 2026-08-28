package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	paymentevents "polyglot-ticketing-v1/contracts/payments"
	"polyglot-ticketing-v1/internal/observability"
)

const (
	paymentFailedDurableName    = "orders-payment-failed-v1"
	paymentSucceededDurableName = "orders-payment-succeeded-v1"
)

type PaymentResultConsumer struct {
	durable   string
	handler   *order.PaymentResultHandler
	logger    *slog.Logger
	metrics   *observability.Metrics
	operation string
	subject   string
	url       string
}

type paymentResultDelivery interface {
	Ack(...nats.AckOpt) error
	Nak(...nats.AckOpt) error
	Term(...nats.AckOpt) error
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
			consumer.handleDelivery(ctx, message.Data, message)
		}
	}
	return context.Canceled
}

func (consumer *PaymentResultConsumer) handleDelivery(ctx context.Context, payload []byte, delivery paymentResultDelivery) {
	started := time.Now()
	err := consumer.handler.Handle(ctx, consumer.subject, payload)
	if err == nil {
		if err := delivery.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge payment result event", "subject", consumer.subject, "error", err)
			consumer.observe("ack_failed", started)
			return
		}
		consumer.observe("success", started)
		return
	}
	if errors.Is(err, order.ErrInvalidPaymentEvent) {
		consumer.logger.Error("terminal payment result event", "subject", consumer.subject, "error", err)
		if termErr := delivery.Term(); termErr != nil {
			consumer.logger.Warn("failed to terminate payment result event", "subject", consumer.subject, "error", termErr)
		}
		consumer.observe("terminal", started)
		return
	}
	consumer.logger.Warn("payment result handling failed; event will be retried", "subject", consumer.subject, "error", err)
	if nakErr := delivery.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge payment result event", "subject", consumer.subject, "error", nakErr)
	}
	consumer.observe("retry", started)
}

func (consumer *PaymentResultConsumer) observe(outcome string, started time.Time) {
	if consumer.metrics != nil {
		consumer.metrics.ObserveBackground("jetstream_consumer", consumer.operation, outcome, started)
	}
}
