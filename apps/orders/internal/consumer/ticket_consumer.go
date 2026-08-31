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
	"polyglot-ticketing-v1/internal/tracing"
)

const (
	durableName       = "orders-ticket-projection-v1"
	streamName        = "TICKETS_EVENTS"
	connectTimeout    = 5 * time.Second
	reconnectInterval = time.Second
)

type TicketConsumer struct {
	handler     *projection.TicketHandler
	deadLetters ticketDeadLetterer
	logger      *slog.Logger
	metrics     *observability.Metrics
	url         string
}

type ticketEventDelivery interface {
	Ack(...nats.AckOpt) error
	Nak(...nats.AckOpt) error
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
	if err := ensureTicketDeadLetterStream(js); err != nil {
		return err
	}
	consumer.deadLetters = jetStreamTicketDeadLetterer{js: js}
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
	ctx = tracing.ExtractNATS(ctx, message.Header)
	metadata, err := message.Metadata()
	if err != nil {
		consumer.logger.Warn("failed to read ticket event delivery metadata; event will be retried", "error", err)
		if nakErr := message.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge ticket event", "error", nakErr)
		}
		return
	}
	consumer.handleDelivery(ctx, ticketDelivery{
		delivery:      message,
		deliveryCount: metadata.NumDelivered,
		headers:       message.Header,
		payload:       message.Data,
		stream:        metadata.Stream,
		streamSeq:     metadata.Sequence.Stream,
		subject:       message.Subject,
	})
}

type ticketDelivery struct {
	delivery      ticketEventDelivery
	deliveryCount uint64
	headers       nats.Header
	payload       []byte
	stream        string
	streamSeq     uint64
	subject       string
}

func (consumer *TicketConsumer) handleDelivery(ctx context.Context, event ticketDelivery) {
	started := time.Now()
	err := consumer.handler.Handle(ctx, event.subject, event.payload)
	if err == nil {
		if err := event.delivery.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge ticket event", "error", err)
			consumer.observe("ack_failed", started)
			return
		}
		consumer.observe("success", started)
		return
	}
	failureClass := ticketFailureClass(err)
	if failureClass == "invalid" || event.deliveryCount > ticketEventMaxRetries {
		if consumer.park(ctx, event, failureClass, err) {
			consumer.observe("dead_lettered", started)
			return
		}
		consumer.observe("dead_letter_publish_failed", started)
		return
	}

	consumer.logger.Warn("ticket projection failed; event will be retried", "subject", event.subject, "error", err)
	if nakErr := event.delivery.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge ticket event", "error", nakErr)
	}
	consumer.observe("retry", started)
}

func (consumer *TicketConsumer) park(ctx context.Context, event ticketDelivery, failureClass string, failure error) bool {
	if consumer.deadLetters == nil {
		consumer.logger.Warn("ticket dead letter queue is unavailable; event will be retried", "subject", event.subject, "error", failure)
		if nakErr := event.delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge ticket event", "error", nakErr)
		}
		return false
	}

	err := consumer.deadLetters.Park(ctx, ticketDeadLetter{
		Consumer:        durableName,
		DeliveryCount:   event.deliveryCount,
		FailureClass:    failureClass,
		FailureReason:   failure.Error(),
		OriginalHeader:  event.headers,
		OriginalStream:  event.stream,
		OriginalSeq:     event.streamSeq,
		OriginalSubject: event.subject,
		Payload:         event.payload,
	})
	if err != nil {
		consumer.logger.Error("failed to park ticket event in dead letter queue; event will be retried", "subject", event.subject, "error", err)
		if nakErr := event.delivery.Nak(); nakErr != nil {
			consumer.logger.Warn("failed to negatively acknowledge ticket event", "error", nakErr)
		}
		return false
	}
	if err := event.delivery.Ack(); err != nil {
		consumer.logger.Warn("failed to acknowledge ticket event after dead-lettering", "error", err)
		return false
	}
	consumer.logger.Warn("ticket event parked in dead letter queue", "subject", event.subject, "stream_sequence", event.streamSeq, "delivery_count", event.deliveryCount, "failure_class", failureClass, "error", failure)
	return true
}

func ticketFailureClass(err error) string {
	if errors.Is(err, order.ErrInvalidTicketEvent) || errors.Is(err, order.ErrUnsupportedSubject) {
		return "invalid"
	}
	if errors.Is(err, order.ErrTicketEventVersionGap) {
		return "version_gap"
	}
	return "retryable"
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
