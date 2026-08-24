package ticket

import (
	"context"
	"strings"
)

const MaxPrice int64 = 9_007_199_254_740_991

type ValidationError struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) Create(ctx context.Context, input CreateInput) (Ticket, []ValidationError, error) {
	if strings.TrimSpace(input.Title) == "" {
		return Ticket{}, []ValidationError{{
			Code:    "INVALID_TITLE",
			Field:   "title",
			Message: "Title is required.",
		}}, nil
	}

	if input.Price <= 0 || input.Price > MaxPrice {
		return Ticket{}, []ValidationError{{
			Code:    "INVALID_PRICE",
			Field:   "price",
			Message: "Price must be between 1 and 9007199254740991.",
		}}, nil
	}

	if input.UserID == "" {
		return Ticket{}, []ValidationError{{
			Code:    "INVALID_ARGUMENT",
			Field:   "userId",
			Message: "Ticket owner is required.",
		}}, nil
	}

	created, err := service.repository.Create(ctx, input)
	return created, nil, err
}

func (service *Service) List(ctx context.Context) ([]Ticket, error) {
	return service.repository.List(ctx)
}

func (service *Service) Get(ctx context.Context, id string) (Ticket, error) {
	return service.repository.FindByID(ctx, id)
}
