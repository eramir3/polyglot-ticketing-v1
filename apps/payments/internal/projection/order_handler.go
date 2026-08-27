package projection

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/payments/internal/payment"
	orderevents "polyglot-ticketing-v1/contracts/orders"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

type OrderHandler struct {
	repository payment.OrderProjectionRepository
}

func NewOrderHandler(repository payment.OrderProjectionRepository) *OrderHandler {
	return &OrderHandler{repository: repository}
}

func (handler *OrderHandler) Handle(ctx context.Context, subject string, payload []byte) error {
	switch subject {
	case orderevents.OrderCreatedSubject:
		return handler.handleCreated(ctx, payload)
	case orderevents.OrderCanceledSubject:
		return handler.handleCanceled(ctx, payload)
	default:
		return payment.ErrUnsupportedSubject
	}
}

func (handler *OrderHandler) handleCreated(ctx context.Context, payload []byte) error {
	var event ordersv1.OrderCreated
	if err := proto.Unmarshal(payload, &event); err != nil {
		return payment.ErrInvalidOrderEvent
	}
	if !validEnvelope(event.GetEventId(), event.GetOccurredAt()) ||
		!validUUID(event.GetOrderId()) || strings.TrimSpace(event.GetUserId()) == "" ||
		event.GetAggregateVersion() < 0 ||
		event.GetOrderStatus() != ordersv1.OrderStatus_ORDER_STATUS_CREATED ||
		event.GetExpiresAt() == nil || event.GetExpiresAt().CheckValid() != nil ||
		event.GetTicket() == nil || !validUUID(event.GetTicket().GetId()) ||
		event.GetTicket().GetPrice() <= 0 {
		return payment.ErrInvalidOrderEvent
	}

	return handler.repository.UpsertOrderFromEvent(ctx, event.GetEventId(), payment.Order{
		AggregateVersion: event.GetAggregateVersion(),
		ID:               event.GetOrderId(),
		Price:            event.GetTicket().GetPrice(),
		Status:           payment.OrderStatusCreated,
		UserID:           event.GetUserId(),
	})
}

func (handler *OrderHandler) handleCanceled(ctx context.Context, payload []byte) error {
	var event ordersv1.OrderCanceled
	if err := proto.Unmarshal(payload, &event); err != nil {
		return payment.ErrInvalidOrderEvent
	}
	if !validEnvelope(event.GetEventId(), event.GetOccurredAt()) ||
		!validUUID(event.GetOrderId()) || event.GetAggregateVersion() < 0 ||
		event.GetTicket() == nil || !validUUID(event.GetTicket().GetId()) {
		return payment.ErrInvalidOrderEvent
	}

	return handler.repository.CancelOrderFromEvent(
		ctx,
		event.GetEventId(),
		event.GetOrderId(),
		event.GetAggregateVersion(),
	)
}

func validEnvelope(eventID string, occurredAt *timestamppb.Timestamp) bool {
	return validUUID(eventID) && occurredAt != nil && occurredAt.CheckValid() == nil
}

func validUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}
