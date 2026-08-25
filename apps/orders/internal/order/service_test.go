package order

import (
	"context"
	"testing"
	"time"
)

func TestServiceCreatesOrderWithFifteenMinuteExpiration(t *testing.T) {
	fixedNow := time.Date(2026, time.August, 25, 19, 0, 0, 0, time.UTC)
	repository := &fakeCreationRepository{}
	service := NewService(repository)
	service.now = func() time.Time { return fixedNow }
	ticketID := "f446d2f3-4515-4b78-8e6a-81797a2517a3"

	_, validationErrors, err := service.Create(context.Background(), ticketID, "user-1")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if len(validationErrors) != 0 {
		t.Fatalf("unexpected validation errors: %+v", validationErrors)
	}
	if repository.input.TicketID != ticketID || repository.input.UserID != "user-1" {
		t.Fatalf("unexpected repository input: %+v", repository.input)
	}
	if want := fixedNow.Add(ExpirationWindow); !repository.input.ExpiresAt.Equal(want) {
		t.Fatalf("expected expiration %s, got %s", want, repository.input.ExpiresAt)
	}
}

func TestServiceRejectsInvalidTicketID(t *testing.T) {
	_, validationErrors, err := NewService(&fakeCreationRepository{}).Create(
		context.Background(),
		"not-a-uuid",
		"user-1",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(validationErrors) != 1 || validationErrors[0].Field != "ticketId" {
		t.Fatalf("expected ticket ID validation error, got %+v", validationErrors)
	}
}

func TestServiceReturnsMissingProjectedTicket(t *testing.T) {
	repository := &fakeCreationRepository{err: ErrNotFound}
	_, validationErrors, err := NewService(repository).Create(
		context.Background(),
		"f446d2f3-4515-4b78-8e6a-81797a2517a3",
		"user-1",
	)
	if len(validationErrors) != 0 {
		t.Fatalf("unexpected validation errors: %+v", validationErrors)
	}
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

type fakeCreationRepository struct {
	err   error
	input CreateInput
}

func (repository *fakeCreationRepository) Create(
	_ context.Context,
	input CreateInput,
) (Order, error) {
	repository.input = input
	return Order{}, repository.err
}
