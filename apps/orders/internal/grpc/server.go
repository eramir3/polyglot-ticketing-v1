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
	reservation, validationErrors, err := server.service.ReserveTicket(
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
	if errors.Is(err, order.ErrReserved) {
		return nil, structuredError(codes.AlreadyExists, []order.ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_ALREADY_EXISTS),
			Message: "Ticket is currently reserved.",
		}})
	}
	if err != nil {
		return nil, structuredError(codes.Internal, []order.ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR),
			Message: "Unable to create order.",
		}})
	}

	return toCreateOrderResponse(reservation), nil
}

func (server *Server) GetOrder(
	ctx context.Context,
	request *ordersv1.GetOrderRequest,
) (*ordersv1.GetOrderResponse, error) {
	found, validationErrors, err := server.service.GetOrder(
		ctx,
		request.GetOrderId(),
		request.GetUserId(),
	)
	if len(validationErrors) > 0 {
		return nil, structuredError(codes.InvalidArgument, validationErrors)
	}
	if errors.Is(err, order.ErrOrderNotFound) {
		return nil, structuredError(codes.NotFound, []order.ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_NOT_FOUND),
			Message: "Order not found.",
		}})
	}
	if err != nil {
		return nil, structuredError(codes.Internal, []order.ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR),
			Message: "Unable to retrieve order.",
		}})
	}

	return &ordersv1.GetOrderResponse{Order: toOrderResponse(found)}, nil
}

func (server *Server) ListOrders(
	ctx context.Context,
	request *ordersv1.ListOrdersRequest,
) (*ordersv1.ListOrdersResponse, error) {
	orders, validationErrors, err := server.service.ListOrders(ctx, request.GetUserId())
	if len(validationErrors) > 0 {
		return nil, structuredError(codes.InvalidArgument, validationErrors)
	}
	if err != nil {
		return nil, structuredError(codes.Internal, []order.ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR),
			Message: "Unable to retrieve orders.",
		}})
	}

	return toListOrdersResponse(orders), nil
}

func toCreateOrderResponse(result order.ReservationResult) *ordersv1.CreateOrderResponse {
	return &ordersv1.CreateOrderResponse{
		Created:   result.Created,
		ExpiresAt: timestamppb.New(result.Order.ExpiresAt),
		Id:        result.Order.ID,
		Status:    toOrderStatus(result.Order.Status),
		TicketId:  result.Order.TicketID,
		UserId:    result.Order.UserID,
	}
}

func toListOrdersResponse(orders []order.Order) *ordersv1.ListOrdersResponse {
	response := &ordersv1.ListOrdersResponse{
		Orders: make([]*ordersv1.Order, 0, len(orders)),
	}
	for _, found := range orders {
		response.Orders = append(response.Orders, toOrderResponse(found))
	}

	return response
}

func toOrderResponse(found order.Order) *ordersv1.Order {
	return &ordersv1.Order{
		ExpiresAt: timestamppb.New(found.ExpiresAt),
		Id:        found.ID,
		Status:    toOrderStatus(found.Status),
		TicketId:  found.TicketID,
		UserId:    found.UserID,
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
