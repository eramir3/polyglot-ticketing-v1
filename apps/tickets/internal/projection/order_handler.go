package projection

import (
	"context"
	"strings"

	"google.golang.org/protobuf/proto"

	"polyglot-ticketing-v1/apps/tickets/internal/ticket"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

type OrderHandler struct {
	repository ticket.ReservationRepository
}

func NewOrderHandler(repository ticket.ReservationRepository) *OrderHandler {
	return &OrderHandler{repository: repository}
}

func (handler *OrderHandler) Handle(ctx context.Context, subject string, payload []byte) error {
	switch subject {
	case orderevents.OrderCreatedSubject:
		return handler.handleCreated(ctx, payload)
	case orderevents.OrderCanceledSubject:
		return handler.handleCanceled(ctx, payload)
	default:
		return ticket.ErrUnsupportedOrderEvent
	}
}

func (handler *OrderHandler) handleCreated(ctx context.Context, payload []byte) error {
	var event ordersv1.OrderCreated
	if err := proto.Unmarshal(payload, &event); err != nil {
		return ticket.ErrInvalidOrderEvent
	}
	if strings.TrimSpace(event.GetEventId()) == "" ||
		event.GetOccurredAt() == nil || event.GetOccurredAt().CheckValid() != nil ||
		strings.TrimSpace(event.GetOrderId()) == "" ||
		strings.TrimSpace(event.GetUserId()) == "" ||
		event.GetAggregateVersion() < 0 ||
		event.GetOrderStatus() != ordersv1.OrderStatus_ORDER_STATUS_CREATED ||
		event.GetExpiresAt() == nil || event.GetExpiresAt().CheckValid() != nil ||
		event.GetTicket() == nil || strings.TrimSpace(event.GetTicket().GetId()) == "" ||
		event.GetTicket().GetPrice() <= 0 {
		return ticket.ErrInvalidOrderEvent
	}

	return handler.repository.ReserveTicketFromOrder(
		ctx,
		event.GetEventId(),
		event.GetOrderId(),
		event.GetTicket().GetId(),
	)
}

func (handler *OrderHandler) handleCanceled(ctx context.Context, payload []byte) error {
	var event ordersv1.OrderCanceled
	if err := proto.Unmarshal(payload, &event); err != nil {
		return ticket.ErrInvalidOrderEvent
	}
	if strings.TrimSpace(event.GetEventId()) == "" ||
		event.GetOccurredAt() == nil || event.GetOccurredAt().CheckValid() != nil ||
		strings.TrimSpace(event.GetOrderId()) == "" ||
		event.GetAggregateVersion() < 0 ||
		event.GetTicket() == nil || strings.TrimSpace(event.GetTicket().GetId()) == "" {
		return ticket.ErrInvalidOrderEvent
	}

	return handler.repository.UnreserveTicketFromOrder(
		ctx,
		event.GetEventId(),
		event.GetOrderId(),
		event.GetTicket().GetId(),
	)
}
