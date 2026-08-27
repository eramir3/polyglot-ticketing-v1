package order

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	paymentevents "polyglot-ticketing-v1/contracts/payments"
	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

// PaymentResultHandler validates payment result events before applying their
// Orders-owned state transitions.
type PaymentResultHandler struct {
	repository PaymentResultEventRepository
}

func NewPaymentResultHandler(repository PaymentResultEventRepository) *PaymentResultHandler {
	return &PaymentResultHandler{repository: repository}
}

func (handler *PaymentResultHandler) Handle(ctx context.Context, subject string, payload []byte) error {
	switch subject {
	case paymentevents.PaymentSucceededSubject:
		return handler.handleSucceeded(ctx, payload)
	case paymentevents.PaymentFailedSubject:
		return handler.handleFailed(ctx, payload)
	default:
		return ErrInvalidPaymentEvent
	}
}

func (handler *PaymentResultHandler) handleSucceeded(ctx context.Context, payload []byte) error {
	var event paymentsv1.PaymentSucceeded
	if err := proto.Unmarshal(payload, &event); err != nil || !validPaymentEvent(event.GetEventId(), event.GetOccurredAt(), event.GetPaymentId(), event.GetOrderId()) {
		return ErrInvalidPaymentEvent
	}
	return handler.repository.ApplyPaymentSucceeded(ctx, event.GetEventId(), event.GetOrderId())
}

func (handler *PaymentResultHandler) handleFailed(ctx context.Context, payload []byte) error {
	var event paymentsv1.PaymentFailed
	if err := proto.Unmarshal(payload, &event); err != nil || !validPaymentEvent(event.GetEventId(), event.GetOccurredAt(), event.GetPaymentId(), event.GetOrderId()) {
		return ErrInvalidPaymentEvent
	}
	return handler.repository.ApplyPaymentFailed(ctx, event.GetEventId(), event.GetOrderId())
}

func validPaymentEvent(eventID string, occurredAt *timestamppb.Timestamp, paymentID, orderID string) bool {
	if _, err := uuid.Parse(eventID); err != nil {
		return false
	}
	if _, err := uuid.Parse(paymentID); err != nil {
		return false
	}
	if _, err := uuid.Parse(orderID); err != nil {
		return false
	}
	return strings.TrimSpace(eventID) != "" &&
		strings.TrimSpace(paymentID) != "" &&
		strings.TrimSpace(orderID) != "" &&
		occurredAt != nil && occurredAt.CheckValid() == nil
}
