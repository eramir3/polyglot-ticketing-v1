package grpc

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"polyglot-ticketing-v1/apps/tickets/internal/ticket"
	ticketsv1 "polyglot-ticketing-v1/protogen/go/tickets/v1"
)

func TestCreateTicketLogsUnexpectedFailure(t *testing.T) {
	var logs bytes.Buffer
	server := NewServer(
		ticket.NewService(failingTicketRepository{}),
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	_, err := server.CreateTicket(context.Background(), &ticketsv1.CreateTicketRequest{
		Title:  "Concert ticket",
		Price:  10_000,
		UserId: "user-1",
	})

	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal status, got %v", status.Code(err))
	}
	output := logs.String()
	for _, expected := range []string{
		"level=ERROR",
		"msg=\"ticket creation failed\"",
		"operation=create_ticket",
		"error=\"database unavailable\"",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected log output to contain %q, got %q", expected, output)
		}
	}
}

func TestUpdateTicketLogsUnexpectedFailure(t *testing.T) {
	var logs bytes.Buffer
	server := NewServer(
		ticket.NewService(failingTicketUpdateRepository{}),
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	_, err := server.UpdateTicket(context.Background(), &ticketsv1.UpdateTicketRequest{
		Id:     "ticket-1",
		Title:  "Updated concert ticket",
		Price:  10_000,
		UserId: "user-1",
	})

	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal status, got %v", status.Code(err))
	}
	output := logs.String()
	for _, expected := range []string{
		"level=ERROR",
		"msg=\"ticket update failed\"",
		"operation=update_ticket",
		"ticket_id=ticket-1",
		"error=\"database unavailable\"",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected log output to contain %q, got %q", expected, output)
		}
	}
}

type failingTicketRepository struct{}

func (failingTicketRepository) Create(context.Context, ticket.CreateInput) (ticket.Ticket, error) {
	return ticket.Ticket{}, errors.New("database unavailable")
}

func (failingTicketRepository) FindByID(context.Context, string) (ticket.Ticket, error) {
	return ticket.Ticket{}, ticket.ErrNotFound
}

func (failingTicketRepository) List(context.Context) ([]ticket.Ticket, error) {
	return nil, nil
}

func (failingTicketRepository) Update(context.Context, string, ticket.UpdateInput) (ticket.Ticket, error) {
	return ticket.Ticket{}, ticket.ErrNotFound
}

type failingTicketUpdateRepository struct {
	failingTicketRepository
}

func (failingTicketUpdateRepository) FindByID(context.Context, string) (ticket.Ticket, error) {
	return ticket.Ticket{ID: "ticket-1", UserID: "user-1"}, nil
}

func (failingTicketUpdateRepository) Update(context.Context, string, ticket.UpdateInput) (ticket.Ticket, error) {
	return ticket.Ticket{}, errors.New("database unavailable")
}
