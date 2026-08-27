package consumer

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/payments/internal/payment"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

func TestOrderConsumerAcknowledgesValidOrderEvent(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderConsumer(&fakeOrderProjectionRepository{}, "", testLogger())

	consumer.handleDelivery(context.Background(), orderevents.OrderCreatedSubject, validOrderCreatedPayload(t), delivery)

	assertDelivery(t, delivery, 1, 0, 0)
}

func TestOrderConsumerNegativeAcknowledgesRetryableEvent(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderConsumer(&fakeOrderProjectionRepository{err: errors.New("database unavailable")}, "", testLogger())

	consumer.handleDelivery(context.Background(), orderevents.OrderCreatedSubject, validOrderCreatedPayload(t), delivery)

	assertDelivery(t, delivery, 0, 1, 0)
}

func TestOrderConsumerTerminatesInvalidEvent(t *testing.T) {
	delivery := &fakeOrderEventDelivery{}
	consumer := NewOrderConsumer(&fakeOrderProjectionRepository{}, "", testLogger())

	consumer.handleDelivery(context.Background(), orderevents.OrderCreatedSubject, nil, delivery)

	assertDelivery(t, delivery, 0, 0, 1)
}

func validOrderCreatedPayload(t *testing.T) []byte {
	t.Helper()
	payload, err := proto.Marshal(&ordersv1.OrderCreated{
		EventId:          uuid.NewString(),
		OccurredAt:       timestamppb.New(time.Now().UTC()),
		OrderId:          uuid.NewString(),
		OrderStatus:      ordersv1.OrderStatus_ORDER_STATUS_CREATED,
		UserId:           "user-1",
		ExpiresAt:        timestamppb.New(time.Now().UTC().Add(time.Minute)),
		AggregateVersion: 0,
		Ticket:           &ordersv1.OrderTicket{Id: uuid.NewString(), Price: 10_000},
	})
	if err != nil {
		t.Fatalf("marshal order-created event: %v", err)
	}
	return payload
}

func assertDelivery(t *testing.T, delivery *fakeOrderEventDelivery, acknowledged, negativelyAcknowledged, terminated int) {
	t.Helper()
	if delivery.acknowledged != acknowledged || delivery.negativelyAcknowledged != negativelyAcknowledged || delivery.terminated != terminated {
		t.Fatalf("unexpected event acknowledgement: %+v", delivery)
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeOrderProjectionRepository struct {
	err error
}

func (repository *fakeOrderProjectionRepository) CancelOrderFromEvent(_ context.Context, _ string, _ string, _ int64) error {
	return repository.err
}

func (repository *fakeOrderProjectionRepository) UpsertOrderFromEvent(_ context.Context, _ string, _ payment.Order) error {
	return repository.err
}

type fakeOrderEventDelivery struct {
	acknowledged           int
	negativelyAcknowledged int
	terminated             int
}

func (delivery *fakeOrderEventDelivery) Ack(...nats.AckOpt) error {
	delivery.acknowledged++
	return nil
}

func (delivery *fakeOrderEventDelivery) Nak(...nats.AckOpt) error {
	delivery.negativelyAcknowledged++
	return nil
}

func (delivery *fakeOrderEventDelivery) Term(...nats.AckOpt) error {
	delivery.terminated++
	return nil
}
