package payment

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"polyglot-ticketing-v1/apps/payments/internal/errorcode"
	commonv1 "polyglot-ticketing-v1/protogen/go/common/v1"
)

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func (service *Service) CreatePayment(
	ctx context.Context,
	input CreateInput,
) (Payment, bool, []ValidationError, error) {
	if validationErrors := validateCreate(input); len(validationErrors) > 0 {
		return Payment{}, false, validationErrors, nil
	}

	payment, created, err := service.repository.Create(ctx, input)
	return payment, created, nil, err
}

func validateCreate(input CreateInput) []ValidationError {
	if _, err := uuid.Parse(input.OrderID); err != nil {
		return []ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT),
			Field:   "orderId",
			Message: "Order ID must be a valid UUID.",
		}}
	}
	if strings.TrimSpace(input.UserID) == "" {
		return []ValidationError{{
			Code:    errorcode.String(commonv1.ErrorCode_ERROR_CODE_INVALID_ARGUMENT),
			Field:   "userId",
			Message: "Payment user is required.",
		}}
	}

	return nil
}
