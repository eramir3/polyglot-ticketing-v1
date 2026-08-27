package ticket

import (
	"context"
	"errors"
)

var (
	ErrForbidden               = errors.New("ticket access forbidden")
	ErrInvalidOrderEvent       = errors.New("invalid order event")
	ErrNotFound                = errors.New("ticket not found")
	ErrOrderReservationPending = errors.New("order reservation is not available yet")
	ErrReserved                = errors.New("ticket is reserved")
	ErrUnsupportedOrderEvent   = errors.New("unsupported order event subject")
)

type Ticket struct {
	ID                string
	Title             string
	Price             int64
	ReservedByOrderID string
	UserID            string
}

type CreateInput struct {
	Title  string
	Price  int64
	UserID string
}

type UpdateInput struct {
	Title  string
	Price  int64
	UserID string
}

type Repository interface {
	Create(context.Context, CreateInput) (Ticket, error)
	FindByID(context.Context, string) (Ticket, error)
	List(context.Context) ([]Ticket, error)
	Update(context.Context, string, UpdateInput) (Ticket, error)
}

type ReservationRepository interface {
	ReserveTicketFromOrder(context.Context, string, string, string) error
	UnreserveTicketFromOrder(context.Context, string, string, string) error
}
