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
	paymentCreatedDurableName = "orders-payment-created-v1"
	paymentEventsStreamName   = "PAYMENTS_EVENTS"
)

type PaymentCreatedConsumer struct {
	handler *order.PaymentCreatedHandler
	logger  *slog.Logger
	metrics *observability.Metrics
	url     string
}

type paymentEventDelivery interface {
	Ack(...nats.AckOpt) error
	Nak(...nats.AckOpt) error
	Term(...nats.AckOpt) error
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
			consumer.handleDelivery(ctx, message.Data, message)
		}
	}
	return context.Canceled
}

func (consumer *PaymentCreatedConsumer) handleDelivery(
	ctx context.Context,
	payload []byte,
	delivery paymentEventDelivery,
) {
	started := time.Now()
	err := consumer.handler.Handle(ctx, payload)
	if err == nil {
		if err := delivery.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge payment-created event", "error", err)
			consumer.observe("ack_failed", started)
			return
		}
		consumer.observe("success", started)
		return
	}
	if errors.Is(err, order.ErrInvalidPaymentEvent) {
		consumer.logger.Error("terminal payment-created event", "error", err)
		if termErr := delivery.Term(); termErr != nil {
			consumer.logger.Warn("failed to terminate payment-created event", "error", termErr)
		}
		consumer.observe("terminal", started)
		return
	}

	consumer.logger.Warn("payment-created handling failed; event will be retried", "error", err)
	if nakErr := delivery.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge payment-created event", "error", nakErr)
	}
	consumer.observe("retry", started)
}

func (consumer *PaymentCreatedConsumer) observe(outcome string, started time.Time) {
	if consumer.metrics != nil {
		consumer.metrics.ObserveBackground("jetstream_consumer", "payment_created", outcome, started)
	}
}
