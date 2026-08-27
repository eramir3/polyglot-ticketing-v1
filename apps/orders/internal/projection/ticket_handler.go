package projection

import (
	"context"
	"strings"

	"google.golang.org/protobuf/proto"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	ticketevents "polyglot-ticketing-v1/contracts/tickets"
	ticketsv1 "polyglot-ticketing-v1/protogen/go/tickets/v1"
)

type TicketHandler struct {
	repository order.TicketProjectionRepository
}

func NewTicketHandler(repository order.TicketProjectionRepository) *TicketHandler {
	return &TicketHandler{repository: repository}
}

func (handler *TicketHandler) Handle(ctx context.Context, subject string, payload []byte) error {
	switch subject {
	case ticketevents.TicketCreatedSubject:
		var event ticketsv1.TicketCreated
		if err := proto.Unmarshal(payload, &event); err != nil {
			return order.ErrInvalidTicketEvent
		}
		return handler.apply(ctx, event.GetEventId(), event.GetAggregateVersion(), event.GetTicket())
	case ticketevents.TicketUpdatedSubject:
		var event ticketsv1.TicketUpdated
		if err := proto.Unmarshal(payload, &event); err != nil {
			return order.ErrInvalidTicketEvent
		}
		return handler.apply(ctx, event.GetEventId(), event.GetAggregateVersion(), event.GetTicket())
	default:
		return order.ErrUnsupportedSubject
	}
}

func (handler *TicketHandler) apply(
	ctx context.Context,
	eventID string,
	aggregateVersion int64,
	ticket *ticketsv1.Ticket,
) error {
	if strings.TrimSpace(eventID) == "" || ticket == nil ||
		strings.TrimSpace(ticket.GetId()) == "" ||
		strings.TrimSpace(ticket.GetTitle()) == "" || ticket.GetPrice() <= 0 ||
		aggregateVersion < 0 {
		return order.ErrInvalidTicketEvent
	}

	return handler.repository.UpsertTicketFromEvent(ctx, eventID, order.Ticket{
		AggregateVersion: aggregateVersion,
		ID:               ticket.GetId(),
		Title:            ticket.GetTitle(),
		Price:            ticket.GetPrice(),
	})
}
