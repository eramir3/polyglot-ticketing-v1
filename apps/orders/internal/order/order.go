package order

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidTicketEvent = errors.New("invalid ticket event")
	ErrNotFound           = errors.New("ticket not found")
	ErrUnsupportedSubject = errors.New("unsupported ticket event subject")
)

const ExpirationWindow = 15 * time.Minute

type Status string

const (
	StatusCreated         Status = "Created"
	StatusCanceled        Status = "Canceled"
	StatusAwaitingPayment Status = "AwaitingPayment"
	StatusComplete        Status = "Complete"
)

type Ticket struct {
	ID    string
	Title string
	Price int64
}

type Order struct {
	ExpiresAt time.Time
	ID        string
	Status    Status
	TicketID  string
	UserID    string
}

type CreateInput struct {
	ExpiresAt time.Time
	TicketID  string
	UserID    string
}

type ValidationError struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type CreationRepository interface {
	Create(context.Context, CreateInput) (Order, error)
}

type TicketProjectionRepository interface {
	UpsertTicketFromEvent(context.Context, string, Ticket) error
}
