package grpc

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"polyglot-ticketing-v1/apps/orders/internal/order"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

const (
	orderID  = "f446d2f3-4515-4b78-8e6a-81797a2517a3"
	ticketID = "9d7d8e0a-7b31-4dd0-8fd8-1a7773f3df7d"
	userID   = "user-1"
)

func TestCreateOrderLogsUnexpectedFailure(t *testing.T) {
	var logs bytes.Buffer
	server := NewServer(
		order.NewService(failingOrderRepository{}),
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	_, err := server.CreateOrder(context.Background(), &ordersv1.CreateOrderRequest{
		TicketId: ticketID,
		UserId:   userID,
	})

	assertInternalErrorLog(t, err, logs.String(), "order creation failed", "operation=create_order", "ticket_id="+ticketID)
}

func TestCancelOrderLogsUnexpectedFailure(t *testing.T) {
	var logs bytes.Buffer
	server := NewServer(
		order.NewService(failingOrderRepository{}),
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	_, err := server.CancelOrder(context.Background(), &ordersv1.CancelOrderRequest{
		OrderId: orderID,
		UserId:  userID,
	})

	assertInternalErrorLog(t, err, logs.String(), "order cancellation failed", "operation=cancel_order", "order_id="+orderID)
}

func assertInternalErrorLog(t *testing.T, err error, logs string, message string, fields ...string) {
	t.Helper()
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal status, got %v", status.Code(err))
	}

	for _, expected := range append([]string{
		"level=ERROR",
		"msg=\"" + message + "\"",
		"error=\"database unavailable\"",
	}, fields...) {
		if !strings.Contains(logs, expected) {
			t.Fatalf("expected log output to contain %q, got %q", expected, logs)
		}
	}
}

type failingOrderRepository struct{}

func (failingOrderRepository) CancelByIDAndUser(context.Context, string, string) (order.Order, error) {
	return order.Order{}, errors.New("database unavailable")
}

func (failingOrderRepository) GetByIDAndUser(context.Context, string, string) (order.Order, error) {
	return order.Order{}, errors.New("database unavailable")
}

func (failingOrderRepository) ListByUser(context.Context, string) ([]order.Order, error) {
	return nil, errors.New("database unavailable")
}

func (failingOrderRepository) ReserveTicket(context.Context, order.TicketReservationInput) (order.ReservationResult, error) {
	return order.ReservationResult{}, errors.New("database unavailable")
}
