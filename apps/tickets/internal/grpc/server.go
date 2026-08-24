package grpc

import (
	"context"
	"encoding/json"
	"errors"

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

	return toCreateTicketResponse(created), nil
}

func (server *Server) UpdateTicket(
	ctx context.Context,
	request *ticketsv1.UpdateTicketRequest,
) (*ticketsv1.UpdateTicketResponse, error) {
	updated, validationErrors, err := server.service.Update(ctx, request.GetId(), ticket.UpdateInput{
		Title:  request.GetTitle(),
		Price:  request.GetPrice(),
		UserID: request.GetUserId(),
	})
	if len(validationErrors) > 0 {
		return nil, structuredError(codes.InvalidArgument, validationErrors)
	}
	if errors.Is(err, ticket.ErrForbidden) {
		return nil, structuredError(codes.PermissionDenied, []ticket.ValidationError{{
			Code:    "FORBIDDEN",
			Message: "You do not have permission to update this ticket.",
		}})
	}
	if errors.Is(err, ticket.ErrNotFound) {
		return nil, structuredError(codes.NotFound, []ticket.ValidationError{{
			Code:    "NOT_FOUND",
			Message: "Ticket not found.",
		}})
	}
	if err != nil {
		return nil, structuredError(codes.Internal, []ticket.ValidationError{{
			Code:    "INTERNAL_ERROR",
			Message: "Unable to update ticket.",
		}})
	}

	return &ticketsv1.UpdateTicketResponse{Ticket: toTicketResponse(updated)}, nil
}

func (server *Server) ListTickets(
	ctx context.Context,
	_request *ticketsv1.ListTicketsRequest,
) (*ticketsv1.ListTicketsResponse, error) {
	listed, err := server.service.List(ctx)
	if err != nil {
		return nil, structuredError(codes.Internal, []ticket.ValidationError{{
			Code:    "INTERNAL_ERROR",
			Message: "Unable to retrieve tickets.",
		}})
	}

	response := &ticketsv1.ListTicketsResponse{
		Tickets: make([]*ticketsv1.Ticket, 0, len(listed)),
	}
	for _, listedTicket := range listed {
		response.Tickets = append(response.Tickets, toTicketResponse(listedTicket))
	}

	return response, nil
}

func (server *Server) GetTicket(
	ctx context.Context,
	request *ticketsv1.GetTicketRequest,
) (*ticketsv1.GetTicketResponse, error) {
	found, err := server.service.Get(ctx, request.GetId())
	if errors.Is(err, ticket.ErrNotFound) {
		return nil, structuredError(codes.NotFound, []ticket.ValidationError{{
			Code:    "NOT_FOUND",
			Message: "Ticket not found.",
		}})
	}
	if err != nil {
		return nil, structuredError(codes.Internal, []ticket.ValidationError{{
			Code:    "INTERNAL_ERROR",
			Message: "Unable to retrieve ticket.",
		}})
	}

	return &ticketsv1.GetTicketResponse{Ticket: toTicketResponse(found)}, nil
}

func toCreateTicketResponse(ticket ticket.Ticket) *ticketsv1.CreateTicketResponse {
	return &ticketsv1.CreateTicketResponse{
		Id:     ticket.ID,
		Price:  ticket.Price,
		Title:  ticket.Title,
		UserId: ticket.UserID,
	}
}

func toTicketResponse(ticket ticket.Ticket) *ticketsv1.Ticket {
	return &ticketsv1.Ticket{
		Id:     ticket.ID,
		Price:  ticket.Price,
		Title:  ticket.Title,
		UserId: ticket.UserID,
	}
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
