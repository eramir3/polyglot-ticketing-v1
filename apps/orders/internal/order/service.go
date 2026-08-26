package order

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"polyglot-ticketing-v1/apps/orders/internal/errorcode"
	commonv1 "polyglot-ticketing-v1/protogen/go/common/v1"
)

type Service struct {
	now        func() time.Time
	repository TicketReservationRepository
}

func NewService(repository TicketReservationRepository) *Service {
	return &Service{now: time.Now, repository: repository}
}

func (service *Service) ReserveTicket(
	ctx context.Context,
	ticketID string,
	userID string,
) (ReservationResult, []ValidationError, error) {
	if validationErrors := validateTicketReservation(ticketID, userID); len(validationErrors) > 0 {
		return ReservationResult{}, validationErrors, nil
	}

	reservation, err := service.repository.ReserveTicket(ctx, TicketReservationInput{
		ExpiresAt: service.now().UTC().Add(ExpirationWindow),
		TicketID:  ticketID,
		UserID:    userID,
	})
	return reservation, nil, err
}

func validateTicketReservation(ticketID string, userID string) []ValidationError {
	if _, err := uuid.Parse(ticketID); err != nil {
		return []ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT),
			Field:   "ticketId",
			Message: "Ticket ID must be a valid UUID.",
		}}
	}
	if strings.TrimSpace(userID) == "" {
		return []ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT),
			Field:   "userId",
			Message: "Order user is required.",
		}}
	}

	return nil
}
