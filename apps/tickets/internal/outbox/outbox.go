package outbox

import (
	"context"
	"time"
)

type Event struct {
	EventID string
	Subject string
	Payload []byte
}

type Repository interface {
	ClaimPending(context.Context, int, time.Duration) ([]Event, error)
	MarkPublished(context.Context, string) error
	MarkFailed(context.Context, string, error) error
}
