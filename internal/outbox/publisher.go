package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	pollInterval    = time.Second
	publishTimeout  = 5 * time.Second
	leaseDuration   = 30 * time.Second
	batchSize       = 100
	streamMaxAge    = 7 * 24 * time.Hour
	duplicateWindow = 2 * time.Minute
)

type Publisher struct {
	repository Repository
	config     Config
	url        string
	logger     *slog.Logger

	mu sync.Mutex
	nc *nats.Conn
	js nats.JetStreamContext
}

type eventDispatchError struct {
	event     Event
	operation string
	err       error
}

func (err *eventDispatchError) Error() string {
	return fmt.Sprintf("outbox event %s %s failed: %v", err.event.EventID, err.operation, err.err)
}

func (err *eventDispatchError) Unwrap() error {
	return err.err
}

func NewPublisher(repository Repository, config Config, url string, logger *slog.Logger) *Publisher {
	return &Publisher{repository: repository, config: config, url: url, logger: logger}
}

func (publisher *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	defer publisher.close()

	for {
		if err := publisher.publishPending(ctx); err != nil && !errors.Is(err, context.Canceled) {
			publisher.logPublishPendingError(err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (publisher *Publisher) logPublishPendingError(err error) {
	var dispatchErr *eventDispatchError
	if errors.As(err, &dispatchErr) {
		publisher.logger.Warn(
			"outbox event dispatch failed",
			"operation", dispatchErr.operation,
			"event_id", dispatchErr.event.EventID,
			"subject", dispatchErr.event.Subject,
			"error", dispatchErr.err,
		)
		return
	}

	publisher.logger.Warn("unable to publish pending outbox events", "error", err)
}

func (publisher *Publisher) publishPending(ctx context.Context) error {
	if err := publisher.ensureJetStream(); err != nil {
		return err
	}

	events, err := publisher.repository.ClaimPending(ctx, batchSize, leaseDuration)
	if err != nil {
		return err
	}
	for _, event := range events {
		publishCtx, cancel := context.WithTimeout(ctx, publishTimeout)
		_, publishErr := publisher.js.PublishMsg(&nats.Msg{
			Subject: event.Subject,
			Header:  nats.Header{"Nats-Msg-Id": []string{event.EventID}},
			Data:    event.Payload,
		}, nats.Context(publishCtx))
		cancel()
		if publishErr != nil {
			if err := publisher.repository.MarkFailed(ctx, event.EventID, publishErr); err != nil {
				return &eventDispatchError{event: event, operation: "mark_failed", err: err}
			}
			return &eventDispatchError{event: event, operation: "publish", err: publishErr}
		}
		if err := publisher.repository.MarkPublished(ctx, event.EventID); err != nil {
			return &eventDispatchError{event: event, operation: "mark_published", err: err}
		}
	}
	return nil
}

func (publisher *Publisher) ensureJetStream() error {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if publisher.nc == nil || publisher.nc.Status() != nats.CONNECTED {
		nc, err := nats.Connect(publisher.url, nats.Timeout(publishTimeout))
		if err != nil {
			return err
		}
		if publisher.nc != nil {
			publisher.nc.Close()
		}
		publisher.nc = nc
		publisher.js, err = nc.JetStream()
		if err != nil {
			return err
		}
	}

	if _, err := publisher.js.StreamInfo(publisher.config.StreamName); err == nil {
		return nil
	}

	_, err := publisher.js.AddStream(&nats.StreamConfig{
		Name:       publisher.config.StreamName,
		Subjects:   publisher.config.Subjects,
		Storage:    nats.FileStorage,
		Retention:  nats.LimitsPolicy,
		MaxAge:     streamMaxAge,
		Duplicates: duplicateWindow,
	})
	return err
}

func (publisher *Publisher) close() {
	publisher.mu.Lock()
	defer publisher.mu.Unlock()
	if publisher.nc != nil {
		publisher.nc.Drain()
		publisher.nc.Close()
		publisher.nc = nil
	}
}
