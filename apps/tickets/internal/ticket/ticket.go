package ticket

import "context"

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

type Repository interface {
	Create(context.Context, CreateInput) (Ticket, error)
}
