package projection

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/tickets/internal/ticket"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

func TestOrderHandlerReservesTicketFromOrderCreated(t *testing.T) {
	repository := &fakeOrderReservationRepository{}
	handler := NewOrderHandler(repository)
	payload := marshalOrderEvent(t, &ordersv1.OrderCreated{
		EventId:     "cb33f2be-57b4-4e78-926c-5e1ba53b99d0",
		OccurredAt:  timestamppb.New(time.Now()),
		OrderId:     "f446d2f3-4515-4b78-8e6a-81797a2517a3",
		OrderStatus: ordersv1.OrderStatus_ORDER_STATUS_CREATED,
		UserId:      "user-1",
		ExpiresAt:   timestamppb.New(time.Now().Add(15 * time.Minute)),
		Ticket: &ordersv1.OrderTicket{
			Id:    "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d",
			Price: 10_000,
		},
	})

	if err := handler.Handle(context.Background(), orderevents.OrderCreatedSubject, payload); err != nil {
		t.Fatalf("handle order-created event: %v", err)
	}
	if repository.eventID != "cb33f2be-57b4-4e78-926c-5e1ba53b99d0" ||
		repository.orderID != "f446d2f3-4515-4b78-8e6a-81797a2517a3" ||
		repository.ticketID != "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d" {
		t.Fatalf("unexpected reservation: %+v", repository)
	}
}

func TestOrderHandlerRejectsInvalidOrderCreatedEvent(t *testing.T) {
	err := NewOrderHandler(&fakeOrderReservationRepository{}).Handle(
		context.Background(),
		orderevents.OrderCreatedSubject,
		marshalOrderEvent(t, &ordersv1.OrderCreated{}),
	)
	if !errors.Is(err, ticket.ErrInvalidOrderEvent) {
		t.Fatalf("expected ErrInvalidOrderEvent, got %v", err)
	}
}

func TestOrderHandlerUnreservesTicketFromOrderCanceled(t *testing.T) {
	repository := &fakeOrderReservationRepository{}
	handler := NewOrderHandler(repository)
	payload := marshalOrderEvent(t, &ordersv1.OrderCanceled{
		EventId:    "cb33f2be-57b4-4e78-926c-5e1ba53b99d0",
		OccurredAt: timestamppb.New(time.Now()),
		OrderId:    "f446d2f3-4515-4b78-8e6a-81797a2517a3",
		Ticket: &ordersv1.OrderCanceledTicket{
			Id: "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d",
		},
	})

	if err := handler.Handle(context.Background(), orderevents.OrderCanceledSubject, payload); err != nil {
		t.Fatalf("handle order-canceled event: %v", err)
	}
	if repository.canceledEventID != "cb33f2be-57b4-4e78-926c-5e1ba53b99d0" ||
		repository.canceledOrderID != "f446d2f3-4515-4b78-8e6a-81797a2517a3" ||
		repository.canceledTicketID != "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d" {
		t.Fatalf("unexpected cancellation: %+v", repository)
	}
}

func TestOrderHandlerRejectsInvalidOrderCanceledEvent(t *testing.T) {
	err := NewOrderHandler(&fakeOrderReservationRepository{}).Handle(
		context.Background(),
		orderevents.OrderCanceledSubject,
		marshalOrderEvent(t, &ordersv1.OrderCanceled{}),
	)
	if !errors.Is(err, ticket.ErrInvalidOrderEvent) {
		t.Fatalf("expected ErrInvalidOrderEvent, got %v", err)
	}
}

func TestOrderHandlerRejectsUnsupportedSubject(t *testing.T) {
	err := NewOrderHandler(&fakeOrderReservationRepository{}).Handle(
		context.Background(),
		"orders.order.completed.v1",
		nil,
	)
	if !errors.Is(err, ticket.ErrUnsupportedOrderEvent) {
		t.Fatalf("expected ErrUnsupportedOrderEvent, got %v", err)
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

type fakeOrderReservationRepository struct {
	canceledEventID  string
	canceledOrderID  string
	canceledTicketID string
	eventID          string
	orderID          string
	ticketID         string
}

func (repository *fakeOrderReservationRepository) ReserveTicketFromOrder(
	_ context.Context,
	eventID string,
	orderID string,
	ticketID string,
) error {
	repository.eventID = eventID
	repository.orderID = orderID
	repository.ticketID = ticketID
	return nil
}

func (repository *fakeOrderReservationRepository) UnreserveTicketFromOrder(
	_ context.Context,
	eventID string,
	orderID string,
	ticketID string,
) error {
	repository.canceledEventID = eventID
	repository.canceledOrderID = orderID
	repository.canceledTicketID = ticketID
	return nil
}
