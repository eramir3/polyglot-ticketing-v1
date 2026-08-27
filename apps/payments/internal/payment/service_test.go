package payment

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestServiceCreatesPaymentForValidInput(t *testing.T) {
	orderID := uuid.NewString()
	repository := &fakeRepository{payment: Payment{ID: uuid.NewString(), OrderID: orderID}, created: true}

	created, wasCreated, validationErrors, err := NewService(repository).CreatePayment(context.Background(), CreateInput{
		OrderID: orderID,
		UserID:  "user-1",
	})

	if err != nil || len(validationErrors) != 0 {
		t.Fatalf("create payment: errors=%+v err=%v", validationErrors, err)
	}
	if !wasCreated || created != repository.payment || repository.input.OrderID != orderID || repository.input.UserID != "user-1" {
		t.Fatalf("unexpected payment creation result: payment=%+v created=%t repository=%+v", created, wasCreated, repository)
	}
}

func TestServiceRejectsInvalidPaymentInput(t *testing.T) {
	repository := &fakeRepository{}
	_, _, validationErrors, err := NewService(repository).CreatePayment(context.Background(), CreateInput{
		OrderID: "not-a-uuid",
		UserID:  "user-1",
	})

	if err != nil || len(validationErrors) != 1 || validationErrors[0].Field != "orderId" {
		t.Fatalf("expected order ID validation error, got errors=%+v err=%v", validationErrors, err)
	}
	if repository.called {
		t.Fatal("invalid payment input reached repository")
	}
}

func TestServiceReturnsRepositoryError(t *testing.T) {
	repository := &fakeRepository{err: ErrOrderNotPayable}
	_, _, validationErrors, err := NewService(repository).CreatePayment(context.Background(), CreateInput{
		OrderID: uuid.NewString(),
		UserID:  "user-1",
	})

	if len(validationErrors) != 0 || !errors.Is(err, ErrOrderNotPayable) {
		t.Fatalf("expected payable error, got errors=%+v err=%v", validationErrors, err)
	}
}

type fakeRepository struct {
	called  bool
	created bool
	err     error
	input   CreateInput
	payment Payment
}

func (repository *fakeRepository) Create(_ context.Context, input CreateInput) (Payment, bool, error) {
	repository.called = true
	repository.input = input
	return repository.payment, repository.created, repository.err
}
