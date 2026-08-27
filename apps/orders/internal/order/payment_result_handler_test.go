package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	paymentevents "polyglot-ticketing-v1/contracts/payments"
	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

func TestPaymentResultHandlerAppliesValidEvents(t *testing.T) {
	testCases := []struct {
		name    string
		subject string
		payload func(*testing.T, string, string) []byte
		action  string
	}{
		{name: "succeeded", subject: paymentevents.PaymentSucceededSubject, payload: marshalPaymentSucceeded, action: "succeeded"},
		{name: "failed", subject: paymentevents.PaymentFailedSubject, payload: marshalPaymentFailed, action: "failed"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &fakePaymentResultRepository{}
			eventID := uuid.NewString()
			orderID := uuid.NewString()

			err := NewPaymentResultHandler(repository).Handle(context.Background(), testCase.subject, testCase.payload(t, eventID, orderID))

			if err != nil || repository.action != testCase.action || repository.eventID != eventID || repository.orderID != orderID {
				t.Fatalf("handle payment result: err=%v repository=%+v", err, repository)
			}
		})
	}
}

func TestPaymentResultHandlerRejectsInvalidEvent(t *testing.T) {
	err := NewPaymentResultHandler(&fakePaymentResultRepository{}).Handle(
		context.Background(),
		paymentevents.PaymentSucceededSubject,
		nil,
	)
	if !errors.Is(err, ErrInvalidPaymentEvent) {
		t.Fatalf("expected invalid payment event, got %v", err)
	}
}

func marshalPaymentSucceeded(t *testing.T, eventID, orderID string) []byte {
	t.Helper()
	payload, err := proto.Marshal(&paymentsv1.PaymentSucceeded{
		EventId: eventID, OccurredAt: timestamppb.New(time.Now().UTC()), PaymentId: uuid.NewString(), OrderId: orderID,
	})
	if err != nil {
		t.Fatalf("marshal payment-succeeded: %v", err)
	}
	return payload
}

func marshalPaymentFailed(t *testing.T, eventID, orderID string) []byte {
	t.Helper()
	payload, err := proto.Marshal(&paymentsv1.PaymentFailed{
		EventId: eventID, OccurredAt: timestamppb.New(time.Now().UTC()), PaymentId: uuid.NewString(), OrderId: orderID,
	})
	if err != nil {
		t.Fatalf("marshal payment-failed: %v", err)
	}
	return payload
}

type fakePaymentResultRepository struct {
	action  string
	err     error
	eventID string
	orderID string
}

func (repository *fakePaymentResultRepository) ApplyPaymentSucceeded(_ context.Context, eventID, orderID string) error {
	repository.action, repository.eventID, repository.orderID = "succeeded", eventID, orderID
	return repository.err
}

func (repository *fakePaymentResultRepository) ApplyPaymentFailed(_ context.Context, eventID, orderID string) error {
	repository.action, repository.eventID, repository.orderID = "failed", eventID, orderID
	return repository.err
}
