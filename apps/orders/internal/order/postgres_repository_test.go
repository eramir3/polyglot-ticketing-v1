package order

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

func TestMarshalOrderCreatedProducesReservationSnapshot(t *testing.T) {
	occurredAt := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	expiresAt := occurredAt.Add(ExpirationWindow)
	eventID := uuid.NewString()
	payload, err := marshalOrderCreated(eventID, occurredAt, Order{
		ExpiresAt: expiresAt,
		ID:        "f446d2f3-4515-4b78-8e6a-81797a2517a3",
		Status:    StatusCreated,
		TicketID:  "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d",
		UserID:    "user-1",
	}, Ticket{
		ID:    "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d",
		Price: 10_000,
	})
	if err != nil {
		t.Fatalf("marshal order-created event: %v", err)
	}

	var event ordersv1.OrderCreated
	if err := proto.Unmarshal(payload, &event); err != nil {
		t.Fatalf("unmarshal order-created event: %v", err)
	}
	if event.GetEventId() != eventID || !event.GetOccurredAt().AsTime().Equal(occurredAt) {
		t.Fatalf("unexpected event envelope: %+v", &event)
	}
	if event.GetOrderId() != "f446d2f3-4515-4b78-8e6a-81797a2517a3" ||
		event.GetOrderStatus() != ordersv1.OrderStatus_ORDER_STATUS_CREATED ||
		event.GetUserId() != "user-1" || !event.GetExpiresAt().AsTime().Equal(expiresAt) {
		t.Fatalf("unexpected order snapshot: %+v", &event)
	}
	if event.GetTicket().GetId() != "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d" ||
		event.GetTicket().GetPrice() != 10_000 {
		t.Fatalf("unexpected ticket snapshot: %+v", event.GetTicket())
	}
}
