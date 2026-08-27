package ticket

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	ticketsv1 "polyglot-ticketing-v1/protogen/go/tickets/v1"
)

func TestMarshalTicketCreatedProducesTicketSnapshot(t *testing.T) {
	occurredAt := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	eventID := uuid.NewString()
	payload, err := marshalTicketCreated(eventID, occurredAt, Ticket{
		AggregateVersion: 0,
		ID:               "ticket-1",
		Title:            "Concert ticket",
		Price:            100,
		UserID:           "user-1",
	})
	if err != nil {
		t.Fatalf("marshal ticket-created event: %v", err)
	}

	var event ticketsv1.TicketCreated
	if err := proto.Unmarshal(payload, &event); err != nil {
		t.Fatalf("unmarshal ticket-created event: %v", err)
	}
	if event.GetEventId() != eventID {
		t.Fatalf("expected event ID %q, got %q", eventID, event.GetEventId())
	}
	if event.GetAggregateVersion() != 0 {
		t.Fatalf("expected aggregate version 0, got %d", event.GetAggregateVersion())
	}
	if !event.GetOccurredAt().AsTime().Equal(occurredAt) {
		t.Fatalf("unexpected occurrence time: %s", event.GetOccurredAt().AsTime())
	}
	if event.GetTicket().GetId() != "ticket-1" ||
		event.GetTicket().GetTitle() != "Concert ticket" ||
		event.GetTicket().GetPrice() != 100 ||
		event.GetTicket().GetUserId() != "user-1" {
		t.Fatalf("unexpected ticket snapshot: %+v", event.GetTicket())
	}
}

func TestMarshalTicketUpdatedProducesTicketSnapshot(t *testing.T) {
	occurredAt := time.Date(2026, time.August, 25, 12, 0, 0, 0, time.UTC)
	eventID := uuid.NewString()
	payload, err := marshalTicketUpdated(eventID, occurredAt, Ticket{
		AggregateVersion: 1,
		ID:               "ticket-1",
		Title:            "Updated concert ticket",
		Price:            200,
		UserID:           "user-1",
	})
	if err != nil {
		t.Fatalf("marshal ticket-updated event: %v", err)
	}

	var event ticketsv1.TicketUpdated
	if err := proto.Unmarshal(payload, &event); err != nil {
		t.Fatalf("unmarshal ticket-updated event: %v", err)
	}
	if event.GetEventId() != eventID {
		t.Fatalf("expected event ID %q, got %q", eventID, event.GetEventId())
	}
	if event.GetAggregateVersion() != 1 {
		t.Fatalf("expected aggregate version 1, got %d", event.GetAggregateVersion())
	}
	if !event.GetOccurredAt().AsTime().Equal(occurredAt) {
		t.Fatalf("unexpected occurrence time: %s", event.GetOccurredAt().AsTime())
	}
	if event.GetTicket().GetId() != "ticket-1" ||
		event.GetTicket().GetTitle() != "Updated concert ticket" ||
		event.GetTicket().GetPrice() != 200 ||
		event.GetTicket().GetUserId() != "user-1" {
		t.Fatalf("unexpected ticket snapshot: %+v", event.GetTicket())
	}
}

func TestMarshalTicketUpdatedPreservesReservationChangeVersion(t *testing.T) {
	payload, err := marshalTicketUpdated("reservation-event", time.Now().UTC(), Ticket{
		AggregateVersion: 2,
		ID:               "ticket-1",
		Title:            "Concert ticket",
		Price:            100,
		UserID:           "user-1",
	})
	if err != nil {
		t.Fatalf("marshal reservation ticket-updated event: %v", err)
	}

	var event ticketsv1.TicketUpdated
	if err := proto.Unmarshal(payload, &event); err != nil {
		t.Fatalf("unmarshal reservation ticket-updated event: %v", err)
	}
	if event.GetAggregateVersion() != 2 {
		t.Fatalf("expected aggregate version 2, got %d", event.GetAggregateVersion())
	}
}
