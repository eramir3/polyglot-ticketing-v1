package grpc

import (
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"polyglot-ticketing-v1/apps/orders/internal/errorcode"
	"polyglot-ticketing-v1/apps/orders/internal/order"
	commonv1 "polyglot-ticketing-v1/protogen/go/common/v1"
	ordersv1 "polyglot-ticketing-v1/protogen/go/orders/v1"
)

type Server struct {
	ordersv1.UnimplementedOrdersServiceServer
	service *order.Service
}

func NewServer(service *order.Service) *Server {
	return &Server{service: service}
}

func (server *Server) CreateOrder(
	ctx context.Context,
	request *ordersv1.CreateOrderRequest,
) (*ordersv1.CreateOrderResponse, error) {
	created, validationErrors, err := server.service.Create(
		ctx,
		request.GetTicketId(),
		request.GetUserId(),
	)
	if len(validationErrors) > 0 {
		return nil, structuredError(codes.InvalidArgument, validationErrors)
	}
	if errors.Is(err, order.ErrNotFound) {
		return nil, structuredError(codes.NotFound, []order.ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_NOT_FOUND),
			Message: "Ticket not found.",
		}})
	}
	if err != nil {
		return nil, structuredError(codes.Internal, []order.ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR),
			Message: "Unable to create order.",
		}})
	}

	return toCreateOrderResponse(created), nil
}

func toCreateOrderResponse(created order.Order) *ordersv1.CreateOrderResponse {
	return &ordersv1.CreateOrderResponse{
		ExpiresAt: timestamppb.New(created.ExpiresAt),
		Id:        created.ID,
		Status:    toOrderStatus(created.Status),
		TicketId:  created.TicketID,
		UserId:    created.UserID,
	}
}

func toOrderStatus(value order.Status) ordersv1.OrderStatus {
	switch value {
	case order.StatusCreated:
		return ordersv1.OrderStatus_ORDER_STATUS_CREATED
	case order.StatusCanceled:
		return ordersv1.OrderStatus_ORDER_STATUS_CANCELED
	case order.StatusAwaitingPayment:
		return ordersv1.OrderStatus_ORDER_STATUS_AWAITING_PAYMENT
	case order.StatusComplete:
		return ordersv1.OrderStatus_ORDER_STATUS_COMPLETE
	default:
		return ordersv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

func structuredError(code codes.Code, errors []order.ValidationError) error {
	payload := struct {
		Errors []order.ValidationError `json:"errors"`
	}{Errors: errors}
	details, err := json.Marshal(payload)
	if err != nil {
		return status.Error(codes.Internal, `{"errors":[{"code":"`+errorcode.String(commonv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR)+`","message":"An unexpected error occurred."}]}`)
	}

	return status.Error(code, string(details))
}
