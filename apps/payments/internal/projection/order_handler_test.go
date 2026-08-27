package projection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/payments/internal/payment"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

func TestOrderHandlerProjectsCreatedOrder(t *testing.T) {
	repository := &fakeOrderProjectionRepository{}
	orderID := uuid.NewString()
	payload := marshalOrderEvent(t, &ordersv1.OrderCreated{
		EventId:          uuid.NewString(),
		OccurredAt:       timestamppb.New(time.Now().UTC()),
		OrderId:          orderID,
		OrderStatus:      ordersv1.OrderStatus_ORDER_STATUS_CREATED,
		UserId:           "user-1",
		ExpiresAt:        timestamppb.New(time.Now().UTC().Add(time.Minute)),
		AggregateVersion: 0,
		Ticket: &ordersv1.OrderTicket{
			Id:    uuid.NewString(),
			Price: 10_000,
		},
	})

	err := NewOrderHandler(repository).Handle(context.Background(), orderevents.OrderCreatedSubject, payload)

	if err != nil {
		t.Fatalf("handle order-created: %v", err)
	}
	if repository.created.ID != orderID || repository.created.Status != payment.OrderStatusCreated || repository.created.Price != 10_000 {
		t.Fatalf("unexpected projected order: %+v", repository.created)
	}
}

func TestOrderHandlerProjectsCanceledOrder(t *testing.T) {
	repository := &fakeOrderProjectionRepository{}
	orderID := uuid.NewString()
	payload := marshalOrderEvent(t, &ordersv1.OrderCanceled{
		EventId:          uuid.NewString(),
		OccurredAt:       timestamppb.New(time.Now().UTC()),
		OrderId:          orderID,
		AggregateVersion: 1,
		Ticket:           &ordersv1.OrderCanceledTicket{Id: uuid.NewString()},
	})

	err := NewOrderHandler(repository).Handle(context.Background(), orderevents.OrderCanceledSubject, payload)

	if err != nil {
		t.Fatalf("handle order-canceled: %v", err)
	}
	if repository.canceledOrderID != orderID || repository.canceledVersion != 1 {
		t.Fatalf("unexpected canceled projection input: %+v", repository)
	}
}

func TestOrderHandlerRejectsInvalidOrderEvent(t *testing.T) {
	err := NewOrderHandler(&fakeOrderProjectionRepository{}).Handle(
		context.Background(),
		orderevents.OrderCreatedSubject,
		nil,
	)
	if !errors.Is(err, payment.ErrInvalidOrderEvent) {
		t.Fatalf("expected invalid order event, got %v", err)
	}
}

func marshalOrderEvent(t *testing.T, event proto.Message) []byte {
	t.Helper()
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal order event: %v", err)
	}
	return payload
}

type fakeOrderProjectionRepository struct {
	canceledEventID string
	canceledOrderID string
	canceledVersion int64
	created         payment.Order
}

func (repository *fakeOrderProjectionRepository) CancelOrderFromEvent(
	_ context.Context,
	eventID string,
	orderID string,
	aggregateVersion int64,
) error {
	repository.canceledEventID = eventID
	repository.canceledOrderID = orderID
	repository.canceledVersion = aggregateVersion
	return nil
}

func (repository *fakeOrderProjectionRepository) UpsertOrderFromEvent(
	_ context.Context,
	_ string,
	order payment.Order,
) error {
	repository.created = order
	return nil
}
