package payment

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var (
	ErrInvalidOrderEvent     = errors.New("invalid order event")
	ErrOrderEventVersionGap  = errors.New("order event version gap")
	ErrOrderNotFound         = errors.New("order not found")
	ErrOrderNotPayable       = errors.New("order cannot be paid")
	ErrUnsupportedSubject    = errors.New("unsupported order event subject")
	ErrInvalidOutcome        = errors.New("invalid payment processor outcome")
	ErrInvalidRandomFailures = errors.New("invalid payment processor random failures setting")
)

type OrderStatus string

const (
	OrderStatusAwaitingPayment OrderStatus = "AwaitingPayment"
	OrderStatusCanceled        OrderStatus = "Canceled"
	OrderStatusComplete        OrderStatus = "Complete"
	OrderStatusCreated         OrderStatus = "Created"
)

type Status string

const (
	StatusFailed    Status = "Failed"
	StatusPending   Status = "Pending"
	StatusSucceeded Status = "Succeeded"
)

type ProcessorOutcome string

const (
	ProcessorOutcomeFailure ProcessorOutcome = "failure"
	ProcessorOutcomeSuccess ProcessorOutcome = "success"
)

type Order struct {
	AggregateVersion int64
	ID               string
	Price            int64
	Status           OrderStatus
	UserID           string
}

type Payment struct {
	ID          string
	OrderID     string
	Status      Status
	Traceparent *string
	Tracestate  *string
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

type SettlementRepository interface {
	ResolveNextPending(context.Context, ProcessorOutcome) (Payment, bool, error)
}

type OrderProjectionRepository interface {
	CancelOrderFromEvent(context.Context, string, string, int64) error
	UpsertOrderFromEvent(context.Context, string, Order) error
}

func ParseProcessorOutcome(value string) (ProcessorOutcome, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(ProcessorOutcomeSuccess):
		return ProcessorOutcomeSuccess, nil
	case string(ProcessorOutcomeFailure):
		return ProcessorOutcomeFailure, nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidOutcome, value)
	}
}

// ParseRandomFailures reads the opt-in local simulation mode. An empty value
// keeps the deterministic processor outcome behavior.
func ParseRandomFailures(value string) (bool, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return false, nil
	}

	randomFailures, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%w: %q", ErrInvalidRandomFailures, value)
	}
	return randomFailures, nil
}
