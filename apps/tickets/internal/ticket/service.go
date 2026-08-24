package ticket

import (
	"context"
	"strings"
)

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

	if input.Price <= 0 {
		return Ticket{}, []ValidationError{{
			Code:    "INVALID_PRICE",
			Field:   "price",
			Message: "Price must be a positive integer.",
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
