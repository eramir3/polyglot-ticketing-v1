package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"polyglot-ticketing-v1/apps/payments/internal/payment"
	"polyglot-ticketing-v1/apps/payments/internal/projection"
	"polyglot-ticketing-v1/internal/observability"
	"polyglot-ticketing-v1/internal/tracing"
)

const (
	orderConsumerDurableName = "payments-order-projection-v1"
	orderEventsStreamName    = "ORDERS_EVENTS"
	connectTimeout           = 5 * time.Second
	reconnectInterval        = time.Second
)

type OrderConsumer struct {
	deadLetters paymentsDeadLetterer
	handler     *projection.OrderHandler
	logger      *slog.Logger
	metrics     *observability.Metrics
	url         string
}

type orderEventDelivery interface {
	Ack(...nats.AckOpt) error
	Nak(...nats.AckOpt) error
	Term(...nats.AckOpt) error
}

func NewOrderConsumer(repository payment.OrderProjectionRepository, url string, logger *slog.Logger, observedMetrics ...*observability.Metrics) *OrderConsumer {
	return &OrderConsumer{
		handler: projection.NewOrderHandler(repository),
		logger:  logger,
		metrics: firstMetrics(observedMetrics),
		url:     url,
	}
}

func (consumer *OrderConsumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := consumer.consume(ctx); err != nil && !errors.Is(err, context.Canceled) {
			consumer.logger.Warn("payments order projection consumer stopped", "error", err)
		}
		if !waitForRetry(ctx) {
			return
		}
	}
}

func (consumer *OrderConsumer) consume(ctx context.Context) error {
	nc, err := nats.Connect(consumer.url, nats.Timeout(connectTimeout))
	if err != nil {
		return err
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		return err
	}
	if err := ensurePaymentsDeadLetterStream(js); err != nil {
		return err
	}
	consumer.deadLetters = jetStreamPaymentsDeadLetterer{js: js}
	subscription, err := js.PullSubscribe(
		"orders.>",
		orderConsumerDurableName,
		nats.BindStream(orderEventsStreamName),
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
			consumer.handleMessage(tracing.ExtractNATS(ctx, message.Header), message)
		}
	}
	return context.Canceled
}

func (consumer *OrderConsumer) handleMessage(ctx context.Context, message *nats.Msg) {
	event, err := newPaymentsDelivery(message)
	if err != nil {
		consumer.logger.Warn("failed to read payments order event delivery metadata; event will be retried", "error", err)
		if nakErr := message.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge payments order event", "error", nakErr)
		}
		return
	}
	consumer.handleDelivery(ctx, event)
}

func (consumer *OrderConsumer) handleDelivery(ctx context.Context, event paymentsDelivery) {
	started := time.Now()
	err := consumer.handler.Handle(ctx, event.subject, event.payload)
	if err == nil {
		if err := event.delivery.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge payments order event", "error", err)
			consumer.observe("ack_failed", started)
			return
		}
		consumer.observe("success", started)
		return
	}
	failureClass := paymentOrderFailureClass(err)
	if failureClass == "invalid" || event.deliveryCount > orderEventMaxRetries {
		if consumer.park(ctx, event, failureClass, err) {
			consumer.observe("dead_lettered", started)
			return
		}
		consumer.observe("dead_letter_publish_failed", started)
		return
	}

	consumer.logger.Warn("payments order projection failed; event will be retried", "subject", event.subject, "error", err)
	if nakErr := event.delivery.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge payments order event", "error", nakErr)
	}
	consumer.observe("retry", started)
}

func (consumer *OrderConsumer) park(ctx context.Context, event paymentsDelivery, failureClass string, failure error) bool {
	if consumer.deadLetters == nil {
		consumer.logger.Warn("payments order event dead letter queue is unavailable; event will be retried", "subject", event.subject, "error", failure)
		if nakErr := event.delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge payments order event", "error", nakErr)
		}
		return false
	}

	if err := consumer.deadLetters.Park(ctx, event.deadLetter(orderConsumerDurableName, failureClass, failure)); err != nil {
		consumer.logger.Error("failed to park payments order event in dead letter queue; event will be retried", "subject", event.subject, "error", err)
		if nakErr := event.delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge payments order event", "error", nakErr)
		}
		return false
	}
	if err := event.delivery.Ack(); err != nil {
		consumer.logger.Warn("failed to acknowledge payments order event after dead-lettering", "error", err)
		return false
	}
	consumer.logger.Warn("payments order event parked in dead letter queue", "subject", event.subject, "stream_sequence", event.streamSeq, "delivery_count", event.deliveryCount, "failure_class", failureClass, "error", failure)
	return true
}

func paymentOrderFailureClass(err error) string {
	if errors.Is(err, payment.ErrInvalidOrderEvent) || errors.Is(err, payment.ErrUnsupportedSubject) {
		return "invalid"
	}
	if errors.Is(err, payment.ErrOrderEventVersionGap) {
		return "version_gap"
	}
	return "retryable"
}

func (consumer *OrderConsumer) observe(outcome string, started time.Time) {
	if consumer.metrics != nil {
		consumer.metrics.ObserveBackground("jetstream_consumer", "order_projection", outcome, started)
	}
}

func firstMetrics(metrics []*observability.Metrics) *observability.Metrics {
	if len(metrics) == 0 {
		return nil
	}
	return metrics[0]
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
