package consumer

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	expirationevents "polyglot-ticketing-v1/contracts/expiration"
)

const (
	expirationCompleteDurableName = "orders-expiration-complete-v1"
	expirationEventsStreamName    = "EXPIRATION_EVENTS"
)

type ExpirationCompleteConsumer struct {
	handler *order.ExpirationCompleteHandler
	logger  *slog.Logger
	url     string
}

type expirationEventDelivery interface {
	Ack(...nats.AckOpt) error
	Nak(...nats.AckOpt) error
	Term(...nats.AckOpt) error
}

func NewExpirationCompleteConsumer(
	repository order.ExpirationEventRepository,
	url string,
	logger *slog.Logger,
) *ExpirationCompleteConsumer {
	return &ExpirationCompleteConsumer{
		handler: order.NewExpirationCompleteHandler(repository),
		logger:  logger,
		url:     url,
	}
}

func (consumer *ExpirationCompleteConsumer) Run(ctx context.Context) {
	for ctx.Err() == nil {
		if err := consumer.consume(ctx); err != nil && !errors.Is(err, context.Canceled) {
			consumer.logger.Warn("expiration-complete consumer stopped", "error", err)
		}
		if !waitForRetry(ctx) {
			return
		}
	}
}

func (consumer *ExpirationCompleteConsumer) consume(ctx context.Context) error {
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
		expirationevents.ExpirationCompleteSubject,
		expirationCompleteDurableName,
		nats.BindStream(expirationEventsStreamName),
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

func (consumer *ExpirationCompleteConsumer) handleDelivery(
	ctx context.Context,
	payload []byte,
	delivery expirationEventDelivery,
) {
	err := consumer.handler.Handle(ctx, payload)
	if err == nil {
		if err := delivery.Ack(); err != nil {
			consumer.logger.Warn("failed to acknowledge expiration-complete event", "error", err)
		}
		return
	}
	if errors.Is(err, order.ErrInvalidExpirationEvent) {
		consumer.logger.Error("terminal expiration-complete event", "error", err)
		if termErr := delivery.Term(); termErr != nil {
			consumer.logger.Warn("failed to terminate expiration-complete event", "error", termErr)
		}
		return
	}

	consumer.logger.Warn("expiration-complete handling failed; event will be retried", "error", err)
	if nakErr := delivery.Nak(); nakErr != nil {
		consumer.logger.Warn("failed to negatively acknowledge expiration-complete event", "error", nakErr)
	}
}
