package outbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"polyglot-ticketing-v1/internal/observability"
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
	metrics    *observability.Metrics

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

func NewPublisher(repository Repository, config Config, url string, logger *slog.Logger, metrics ...*observability.Metrics) *Publisher {
	publisher := &Publisher{repository: repository, config: config, url: url, logger: logger}
	if len(metrics) > 0 {
		publisher.metrics = metrics[0]
	}
	return publisher
}

func (publisher *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	defer publisher.close()

	for {
		started := time.Now()
		if err := publisher.publishPending(ctx); err != nil && !errors.Is(err, context.Canceled) {
			publisher.logPublishPendingError(err, started)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (publisher *Publisher) logPublishPendingError(err error, started time.Time) {
	var dispatchErr *eventDispatchError
	if errors.As(err, &dispatchErr) {
		publisher.logger.Warn(
			"outbox event dispatch failed",
			"operation", dispatchErr.operation,
			"event_id", dispatchErr.event.EventID,
			"subject", dispatchErr.event.Subject,
			"error", dispatchErr.err,
		)
		publisher.observe(dispatchErr.operation, "failure", started)
		return
	}

	publisher.logger.Warn("unable to publish pending outbox events", "error", err)
	publisher.observe("poll", "failure", started)
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
		started := time.Now()
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
		publisher.observe("publish", "success", started)
	}
	return nil
}

func (publisher *Publisher) observe(operation string, outcome string, started time.Time) {
	if publisher.metrics != nil {
		publisher.metrics.ObserveBackground("outbox", operation, outcome, started)
	}
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
