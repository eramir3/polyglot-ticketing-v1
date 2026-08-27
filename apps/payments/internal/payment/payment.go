package payment

import (
	"context"
	"errors"
)

var (
	ErrInvalidOrderEvent    = errors.New("invalid order event")
	ErrOrderEventVersionGap = errors.New("order event version gap")
	ErrOrderNotFound        = errors.New("order not found")
	ErrOrderNotPayable      = errors.New("order cannot be paid")
	ErrUnsupportedSubject   = errors.New("unsupported order event subject")
)

type OrderStatus string

const (
	OrderStatusAwaitingPayment OrderStatus = "AwaitingPayment"
	OrderStatusCanceled        OrderStatus = "Canceled"
	OrderStatusComplete        OrderStatus = "Complete"
	OrderStatusCreated         OrderStatus = "Created"
)

type Order struct {
	AggregateVersion int64
	ID               string
	Price            int64
	Status           OrderStatus
	UserID           string
}

type Payment struct {
	ID      string
	OrderID string
}

type CreateInput struct {
	OrderID string
	UserID  string
}

type ValidationError struct {
	Code    string `json:"code"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message"`
}

type Repository interface {
	Create(context.Context, CreateInput) (Payment, bool, error)
}

type OrderProjectionRepository interface {
	CancelOrderFromEvent(context.Context, string, string, int64) error
	UpsertOrderFromEvent(context.Context, string, Order) error
}
