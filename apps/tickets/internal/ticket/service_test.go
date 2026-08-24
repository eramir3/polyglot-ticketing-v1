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

type fakeRepository struct{}

func (fakeRepository) Create(_ context.Context, input CreateInput) (Ticket, error) {
	return Ticket{ID: "ticket-1", Title: input.Title, Price: input.Price, UserID: input.UserID}, nil
}
