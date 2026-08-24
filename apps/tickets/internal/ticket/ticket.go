package ticket

import (
	"context"
	"errors"
)

var (
	ErrForbidden = errors.New("ticket access forbidden")
	ErrNotFound  = errors.New("ticket not found")
)

type Ticket struct {
	ID     string
	Title  string
	Price  int64
	UserID string
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
