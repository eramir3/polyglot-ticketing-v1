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
		ID:     "ticket-1",
		Title:  "Concert ticket",
		Price:  100,
		UserID: "user-1",
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
