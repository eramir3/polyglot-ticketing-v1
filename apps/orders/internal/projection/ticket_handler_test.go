package projection

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	ticketevents "polyglot-ticketing-v1/contracts/tickets"
	ticketsv1 "polyglot-ticketing-v1/protogen/go/tickets/v1"
)

func TestTicketHandlerAppliesCreatedTicket(t *testing.T) {
	repository := &fakeTicketRepository{}
	handler := NewTicketHandler(repository)
	payload := marshalEvent(t, &ticketsv1.TicketCreated{
		EventId:    "cb33f2be-57b4-4e78-926c-5e1ba53b99d0",
		OccurredAt: timestamppb.New(time.Now()),
		Ticket: &ticketsv1.Ticket{
			Id: "f446d2f3-4515-4b78-8e6a-81797a2517a3", Title: "Concert ticket", Price: 100,
		},
	})

	if err := handler.Handle(context.Background(), ticketevents.TicketCreatedSubject, payload); err != nil {
		t.Fatalf("handle ticket-created event: %v", err)
	}
	if repository.eventID != "cb33f2be-57b4-4e78-926c-5e1ba53b99d0" {
		t.Fatalf("unexpected event ID: %q", repository.eventID)
	}
	if repository.ticket != (order.Ticket{ID: "f446d2f3-4515-4b78-8e6a-81797a2517a3", Title: "Concert ticket", Price: 100}) {
		t.Fatalf("unexpected ticket: %+v", repository.ticket)
	}
}

func TestTicketHandlerAppliesUpdatedTicket(t *testing.T) {
	repository := &fakeTicketRepository{}
	handler := NewTicketHandler(repository)
	payload := marshalEvent(t, &ticketsv1.TicketUpdated{
		EventId:    "2b997858-1973-498a-ad25-e5c0620e73fb",
		OccurredAt: timestamppb.New(time.Now()),
		Ticket: &ticketsv1.Ticket{
			Id: "f446d2f3-4515-4b78-8e6a-81797a2517a3", Title: "Updated concert ticket", Price: 200,
		},
	})

	if err := handler.Handle(context.Background(), ticketevents.TicketUpdatedSubject, payload); err != nil {
		t.Fatalf("handle ticket-updated event: %v", err)
	}
	if repository.ticket != (order.Ticket{ID: "f446d2f3-4515-4b78-8e6a-81797a2517a3", Title: "Updated concert ticket", Price: 200}) {
		t.Fatalf("unexpected ticket: %+v", repository.ticket)
	}
}

func TestTicketHandlerRejectsUnsupportedSubject(t *testing.T) {
	err := NewTicketHandler(&fakeTicketRepository{}).Handle(context.Background(), "tickets.ticket.deleted.v1", nil)
	if !errors.Is(err, order.ErrUnsupportedSubject) {
		t.Fatalf("expected ErrUnsupportedSubject, got %v", err)
	}
}

func marshalEvent(t *testing.T, event proto.Message) []byte {
	t.Helper()
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	return payload
}

type fakeTicketRepository struct {
	eventID string
	ticket  order.Ticket
}

func (repository *fakeTicketRepository) UpsertTicketFromEvent(
	_ context.Context,
	eventID string,
	ticket order.Ticket,
) error {
	repository.eventID = eventID
	repository.ticket = ticket
	return nil
}
