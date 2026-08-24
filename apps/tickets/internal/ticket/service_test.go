package ticket

import (
	"context"
	"testing"
)

func TestServiceCreateRejectsInvalidInput(t *testing.T) {
	service := NewService(fakeRepository{})

	testCases := []struct {
		name  string
		input CreateInput
		code  string
		field string
	}{
		{
			name:  "missing title",
			input: CreateInput{Price: 100, UserID: "user-1"},
			code:  "INVALID_TITLE",
			field: "title",
		},
		{
			name:  "non-positive price",
			input: CreateInput{Title: "Concert ticket", UserID: "user-1"},
			code:  "INVALID_PRICE",
			field: "price",
		},
		{
			name:  "price above the JavaScript safe integer limit",
			input: CreateInput{Title: "Concert ticket", Price: MaxPrice + 1, UserID: "user-1"},
			code:  "INVALID_PRICE",
			field: "price",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, validationErrors, err := service.Create(context.Background(), testCase.input)

			if err != nil {
				t.Fatalf("expected no internal error, got %v", err)
			}
			if len(validationErrors) != 1 {
				t.Fatalf("expected one validation error, got %d", len(validationErrors))
			}
			if validationErrors[0].Code != testCase.code || validationErrors[0].Field != testCase.field {
				t.Fatalf("unexpected validation error: %+v", validationErrors[0])
			}
		})
	}
}

func TestServiceCreateAcceptsMaximumSafePrice(t *testing.T) {
	service := NewService(fakeRepository{})

	created, validationErrors, err := service.Create(context.Background(), CreateInput{
		Title:  "Concert ticket",
		Price:  MaxPrice,
		UserID: "user-1",
	})

	if err != nil {
		t.Fatalf("expected no internal error, got %v", err)
	}
	if len(validationErrors) != 0 {
		t.Fatalf("expected no validation errors, got %+v", validationErrors)
	}
	if created.Price != MaxPrice {
		t.Fatalf("expected price %d, got %d", MaxPrice, created.Price)
	}
}

func TestServiceListReturnsRepositoryTickets(t *testing.T) {
	service := NewService(fakeRepository{tickets: []Ticket{{
		ID:     "ticket-1",
		Price:  100,
		Title:  "Concert ticket",
		UserID: "user-1",
	}}})

	listed, err := service.List(context.Background())

	if err != nil {
		t.Fatalf("expected no internal error, got %v", err)
	}
	if len(listed) != 1 || listed[0].ID != "ticket-1" {
		t.Fatalf("unexpected tickets: %+v", listed)
	}
}

type fakeRepository struct {
	tickets []Ticket
}

func (fakeRepository) Create(_ context.Context, input CreateInput) (Ticket, error) {
	return Ticket{ID: "ticket-1", Title: input.Title, Price: input.Price, UserID: input.UserID}, nil
}

func (repository fakeRepository) List(_ context.Context) ([]Ticket, error) {
	return repository.tickets, nil
}
