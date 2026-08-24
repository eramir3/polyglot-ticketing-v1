package grpc

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	ticketsv1 "polyglot-ticketing-v1/apps/tickets/gen/tickets/v1"
	"polyglot-ticketing-v1/apps/tickets/internal/ticket"
)

type Server struct {
	ticketsv1.UnimplementedTicketsServiceServer
	service *ticket.Service
}

func NewServer(service *ticket.Service) *Server {
	return &Server{service: service}
}

func (server *Server) CreateTicket(
	ctx context.Context,
	request *ticketsv1.CreateTicketRequest,
) (*ticketsv1.CreateTicketResponse, error) {
	created, validationErrors, err := server.service.Create(ctx, ticket.CreateInput{
		Title:  request.GetTitle(),
		Price:  request.GetPrice(),
		UserID: request.GetUserId(),
	})
	if len(validationErrors) > 0 {
		return nil, structuredError(codes.InvalidArgument, validationErrors)
	}
	if err != nil {
		return nil, structuredError(codes.Internal, []ticket.ValidationError{{
			Code:    "INTERNAL_ERROR",
			Message: "Unable to create ticket.",
		}})
	}

	return &ticketsv1.CreateTicketResponse{
		Id:     created.ID,
		Price:  created.Price,
		Title:  created.Title,
		UserId: created.UserID,
	}, nil
}

func structuredError(code codes.Code, errors []ticket.ValidationError) error {
	payload := struct {
		Errors []ticket.ValidationError `json:"errors"`
	}{Errors: errors}
	details, err := json.Marshal(payload)
	if err != nil {
		return status.Error(codes.Internal, `{"errors":[{"code":"INTERNAL_ERROR","message":"An unexpected error occurred."}]}`)
	}

	return status.Error(code, string(details))
}
