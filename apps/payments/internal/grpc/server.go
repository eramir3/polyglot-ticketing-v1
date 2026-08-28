package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"polyglot-ticketing-v1/apps/payments/internal/errorcode"
	"polyglot-ticketing-v1/apps/payments/internal/payment"
	commonv1 "polyglot-ticketing-v1/protogen/go/common/v1"
	paymentsv1 "polyglot-ticketing-v1/protogen/go/payments/v1"
)

type Server struct {
	paymentsv1.UnimplementedPaymentsServiceServer
	service *payment.Service
	logger  *slog.Logger
}

func NewServer(service *payment.Service, logger *slog.Logger) *Server {
	return &Server{service: service, logger: logger}
}

func (server *Server) CreatePayment(
	ctx context.Context,
	request *paymentsv1.CreatePaymentRequest,
) (*paymentsv1.CreatePaymentResponse, error) {
	created, wasCreated, validationErrors, err := server.service.CreatePayment(ctx, payment.CreateInput{
		OrderID: request.GetOrderId(),
		UserID:  request.GetUserId(),
	})
	if len(validationErrors) > 0 {
		return nil, structuredError(codes.InvalidArgument, validationErrors)
	}
	if errors.Is(err, payment.ErrOrderNotFound) {
		return nil, structuredError(codes.NotFound, []payment.ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_NOT_FOUND),
			Message: "Order not found.",
		}})
	}
	if errors.Is(err, payment.ErrOrderNotPayable) {
		return nil, structuredError(codes.AlreadyExists, []payment.ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_ALREADY_EXISTS),
			Message: "Order cannot be paid.",
		}})
	}
	if err != nil {
		server.logger.Error(
			"payment creation failed",
			"operation", "create_payment",
			"order_id", request.GetOrderId(),
			"error", err,
		)
		return nil, structuredError(codes.Internal, []payment.ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR),
			Message: "Unable to create payment.",
		}})
	}

	return &paymentsv1.CreatePaymentResponse{
		Created: wasCreated,
		Payment: &paymentsv1.Payment{
			Id:      created.ID,
			OrderId: created.OrderID,
		},
	}, nil
}

func structuredError(code codes.Code, validationErrors []payment.ValidationError) error {
	payload := struct {
		Errors []payment.ValidationError `json:"errors"`
	}{Errors: validationErrors}
	details, err := json.Marshal(payload)
	if err != nil {
		return status.Error(codes.Internal, `{"errors":[{"code":"`+errorcode.String(commonv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR)+`","message":"An unexpected error occurred."}]}`)
	}
	return status.Error(code, string(details))
}
