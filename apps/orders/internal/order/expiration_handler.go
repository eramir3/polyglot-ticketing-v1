package order

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	expirationv1 "polyglot-ticketing-v1/protogen/go/expiration/v1"
)

// ExpirationCompleteHandler validates expiration events before applying their
// state transition through the Orders-owned repository.
type ExpirationCompleteHandler struct {
	repository ExpirationEventRepository
}

func NewExpirationCompleteHandler(repository ExpirationEventRepository) *ExpirationCompleteHandler {
	return &ExpirationCompleteHandler{repository: repository}
}

func (handler *ExpirationCompleteHandler) Handle(ctx context.Context, payload []byte) error {
	var event expirationv1.ExpirationComplete
	if err := proto.Unmarshal(payload, &event); err != nil {
		return ErrInvalidExpirationEvent
	}
	if _, err := uuid.Parse(event.GetEventId()); err != nil {
		return ErrInvalidExpirationEvent
	}
	if _, err := uuid.Parse(event.GetOrderId()); err != nil {
		return ErrInvalidExpirationEvent
	}
	if strings.TrimSpace(event.GetEventId()) == "" ||
		strings.TrimSpace(event.GetOrderId()) == "" ||
		event.GetOccurredAt() == nil || event.GetOccurredAt().CheckValid() != nil {
		return ErrInvalidExpirationEvent
	}

	return handler.repository.ApplyExpirationComplete(ctx, event.GetEventId(), event.GetOrderId())
}
