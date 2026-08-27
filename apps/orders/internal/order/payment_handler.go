package order

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

// PaymentCreatedHandler validates PaymentCreated events before applying their
// state transition through the Orders-owned repository.
type PaymentCreatedHandler struct {
	repository PaymentEventRepository
}

func NewPaymentCreatedHandler(repository PaymentEventRepository) *PaymentCreatedHandler {
	return &PaymentCreatedHandler{repository: repository}
}

func (handler *PaymentCreatedHandler) Handle(ctx context.Context, payload []byte) error {
	var event paymentsv1.PaymentCreated
	if err := proto.Unmarshal(payload, &event); err != nil {
		return ErrInvalidPaymentEvent
	}
	if _, err := uuid.Parse(event.GetEventId()); err != nil {
		return ErrInvalidPaymentEvent
	}
	if _, err := uuid.Parse(event.GetPaymentId()); err != nil {
		return ErrInvalidPaymentEvent
	}
	if _, err := uuid.Parse(event.GetOrderId()); err != nil {
		return ErrInvalidPaymentEvent
	}
	if strings.TrimSpace(event.GetEventId()) == "" ||
		strings.TrimSpace(event.GetPaymentId()) == "" ||
		strings.TrimSpace(event.GetOrderId()) == "" ||
		event.GetOccurredAt() == nil || event.GetOccurredAt().CheckValid() != nil {
		return ErrInvalidPaymentEvent
	}

	return handler.repository.ApplyPaymentCreated(ctx, event.GetEventId(), event.GetOrderId())
}
