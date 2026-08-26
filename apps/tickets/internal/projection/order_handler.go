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
	if subject != orderevents.OrderCreatedSubject {
		return ticket.ErrUnsupportedOrderEvent
	}

	var event ordersv1.OrderCreated
	if err := proto.Unmarshal(payload, &event); err != nil {
		return ticket.ErrInvalidOrderEvent
	}
	if strings.TrimSpace(event.GetEventId()) == "" ||
		strings.TrimSpace(event.GetOrderId()) == "" ||
		strings.TrimSpace(event.GetUserId()) == "" ||
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
