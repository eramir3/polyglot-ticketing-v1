package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

func TestPaymentCreatedHandlerAppliesValidEvent(t *testing.T) {
	repository := &fakePaymentEventRepository{}
	eventID := uuid.NewString()
	orderID := uuid.NewString()
	payload := marshalPaymentCreated(t, &paymentsv1.PaymentCreated{
		EventId:    eventID,
		OccurredAt: timestamppb.New(time.Now().UTC()),
		PaymentId:  uuid.NewString(),
		OrderId:    orderID,
	})

	err := NewPaymentCreatedHandler(repository).Handle(context.Background(), payload)

	if err != nil || repository.eventID != eventID || repository.orderID != orderID {
		t.Fatalf("handle payment-created: err=%v repository=%+v", err, repository)
	}
}

func TestPaymentCreatedHandlerRejectsInvalidEvent(t *testing.T) {
	err := NewPaymentCreatedHandler(&fakePaymentEventRepository{}).Handle(context.Background(), nil)
	if !errors.Is(err, ErrInvalidPaymentEvent) {
		t.Fatalf("expected invalid payment event, got %v", err)
	}
}

func marshalPaymentCreated(t *testing.T, event *paymentsv1.PaymentCreated) []byte {
	t.Helper()
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal payment-created: %v", err)
	}
	return payload
}

type fakePaymentEventRepository struct {
	err     error
	eventID string
	orderID string
}

func (repository *fakePaymentEventRepository) ApplyPaymentCreated(_ context.Context, eventID, orderID string) error {
	repository.eventID = eventID
	repository.orderID = orderID
	return repository.err
}
