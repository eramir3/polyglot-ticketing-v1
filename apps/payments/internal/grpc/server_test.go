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

	"polyglot-ticketing-v1/apps/payments/internal/payment"
	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

func TestCreatePaymentLogsUnexpectedFailure(t *testing.T) {
	const orderID = "f446d2f3-4515-4b78-8e6a-81797a2517a3"

	var logs bytes.Buffer
	server := NewServer(
		payment.NewService(failingPaymentRepository{}),
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	_, err := server.CreatePayment(context.Background(), &paymentsv1.CreatePaymentRequest{
		OrderId: orderID,
		UserId:  "user-1",
	})

	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal status, got %v", status.Code(err))
	}
	for _, expected := range []string{
		"level=ERROR",
		"msg=\"payment creation failed\"",
		"operation=create_payment",
		"order_id=" + orderID,
		"error=\"database unavailable\"",
	} {
		if !strings.Contains(logs.String(), expected) {
			t.Fatalf("expected log output to contain %q, got %q", expected, logs.String())
		}
	}
}

type failingPaymentRepository struct{}

func (failingPaymentRepository) Create(context.Context, payment.CreateInput) (payment.Payment, bool, error) {
	return payment.Payment{}, false, errors.New("database unavailable")
}
