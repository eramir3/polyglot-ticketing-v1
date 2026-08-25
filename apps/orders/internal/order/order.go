package order

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidTicketEvent = errors.New("invalid ticket event")
	ErrUnsupportedSubject = errors.New("unsupported ticket event subject")
)

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

type TicketProjectionRepository interface {
	UpsertTicketFromEvent(context.Context, string, Ticket) error
}
