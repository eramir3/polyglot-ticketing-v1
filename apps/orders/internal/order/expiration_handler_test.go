package order

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	expirationv1 "polyglot-ticketing-v1/protogen/go/expiration/v1"
)

func TestExpirationCompleteHandlerAppliesValidEvent(t *testing.T) {
	eventID := uuid.NewString()
	orderID := uuid.NewString()
	repository := &fakeExpirationEventRepository{}
	payload := marshalExpirationComplete(t, &expirationv1.ExpirationComplete{
		EventId:    eventID,
		OccurredAt: timestamppb.New(time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)),
		OrderId:    orderID,
	})

	err := NewExpirationCompleteHandler(repository).Handle(context.Background(), payload)

	if err != nil {
		t.Fatalf("handle expiration-complete event: %v", err)
	}
	if repository.eventID != eventID || repository.orderID != orderID {
		t.Fatalf("unexpected repository input: %+v", repository)
	}
}

func TestExpirationCompleteHandlerRejectsInvalidEvents(t *testing.T) {
	testCases := []struct {
		name  string
		event *expirationv1.ExpirationComplete
	}{
		{name: "unmarshalable", event: nil},
		{name: "missing envelope", event: &expirationv1.ExpirationComplete{}},
		{name: "missing timestamp", event: &expirationv1.ExpirationComplete{EventId: uuid.NewString(), OrderId: uuid.NewString()}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &fakeExpirationEventRepository{}
			payload := []byte("not a protobuf")
			if testCase.event != nil {
				payload = marshalExpirationComplete(t, testCase.event)
			}

			err := NewExpirationCompleteHandler(repository).Handle(context.Background(), payload)

			if !errors.Is(err, ErrInvalidExpirationEvent) {
				t.Fatalf("expected invalid expiration event, got %v", err)
			}
			if repository.eventID != "" || repository.orderID != "" {
				t.Fatalf("invalid event reached repository: %+v", repository)
			}
		})
	}
}

func marshalExpirationComplete(t *testing.T, event *expirationv1.ExpirationComplete) []byte {
	t.Helper()
	payload, err := proto.Marshal(event)
	if err != nil {
		t.Fatalf("marshal expiration-complete event: %v", err)
	}
	return payload
}

type fakeExpirationEventRepository struct {
	err     error
	eventID string
	orderID string
}

func (repository *fakeExpirationEventRepository) ApplyExpirationComplete(
	_ context.Context,
	eventID string,
	orderID string,
) error {
	repository.eventID = eventID
	repository.orderID = orderID
	return repository.err
}
