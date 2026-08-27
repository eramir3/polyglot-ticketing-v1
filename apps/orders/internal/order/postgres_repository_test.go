package order

import (
	"errors"
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
		AggregateVersion: 0,
		ExpiresAt:        expiresAt,
		ID:               "f446d2f3-4515-4b78-8e6a-81797a2517a3",
		Status:           StatusCreated,
		TicketID:         "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d",
		UserID:           "user-1",
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
		event.GetUserId() != "user-1" || !event.GetExpiresAt().AsTime().Equal(expiresAt) ||
		event.GetAggregateVersion() != 0 {
		t.Fatalf("unexpected order snapshot: %+v", &event)
	}
	if event.GetTicket().GetId() != "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d" ||
		event.GetTicket().GetPrice() != 10_000 {
		t.Fatalf("unexpected ticket snapshot: %+v", event.GetTicket())
	}
}

func TestMarshalOrderCanceledProducesReservationSnapshot(t *testing.T) {
	occurredAt := time.Date(2026, time.August, 26, 12, 0, 0, 0, time.UTC)
	eventID := uuid.NewString()
	payload, err := marshalOrderCanceled(eventID, occurredAt, Order{
		AggregateVersion: 1,
		ID:               "f446d2f3-4515-4b78-8e6a-81797a2517a3",
		Status:           StatusCanceled,
		TicketID:         "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d",
	})
	if err != nil {
		t.Fatalf("marshal order-canceled event: %v", err)
	}

	var event ordersv1.OrderCanceled
	if err := proto.Unmarshal(payload, &event); err != nil {
		t.Fatalf("unmarshal order-canceled event: %v", err)
	}
	if event.GetEventId() != eventID || !event.GetOccurredAt().AsTime().Equal(occurredAt) {
		t.Fatalf("unexpected event envelope: %+v", &event)
	}
	if event.GetOrderId() != "f446d2f3-4515-4b78-8e6a-81797a2517a3" || event.GetAggregateVersion() != 1 {
		t.Fatalf("unexpected order event: %+v", &event)
	}
	if event.GetTicket().GetId() != "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d" {
		t.Fatalf("unexpected ticket snapshot: %+v", event.GetTicket())
	}
}

func TestDecideTicketEventVersion(t *testing.T) {
	testCases := []struct {
		name             string
		hasStoredVersion bool
		storedVersion    int64
		incomingVersion  int64
		wantDecision     ticketEventVersionDecision
		wantErr          error
	}{
		{
			name:            "applies initial version",
			incomingVersion: 0,
			wantDecision:    applyTicketEventVersion,
		},
		{
			name:             "applies next version",
			hasStoredVersion: true,
			storedVersion:    1,
			incomingVersion:  2,
			wantDecision:     applyTicketEventVersion,
		},
		{
			name:            "rejects missing initial version",
			incomingVersion: 1,
			wantErr:         ErrTicketEventVersionGap,
		},
		{
			name:             "rejects skipped version",
			hasStoredVersion: true,
			storedVersion:    1,
			incomingVersion:  3,
			wantErr:          ErrTicketEventVersionGap,
		},
		{
			name:             "ignores duplicate version",
			hasStoredVersion: true,
			storedVersion:    1,
			incomingVersion:  1,
			wantDecision:     ignoreTicketEventVersion,
		},
		{
			name:             "ignores stale version",
			hasStoredVersion: true,
			storedVersion:    2,
			incomingVersion:  1,
			wantDecision:     ignoreTicketEventVersion,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			decision, err := decideTicketEventVersion(
				testCase.hasStoredVersion,
				testCase.storedVersion,
				testCase.incomingVersion,
			)

			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("expected error %v, got %v", testCase.wantErr, err)
			}
			if err == nil && decision != testCase.wantDecision {
				t.Fatalf("expected decision %d, got %d", testCase.wantDecision, decision)
			}
		})
	}
}
