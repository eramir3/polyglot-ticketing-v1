package outbox

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	streamName      = "TICKETS_EVENTS"
	pollInterval    = time.Second
	publishTimeout  = 5 * time.Second
	leaseDuration   = 30 * time.Second
	batchSize       = 100
	streamMaxAge    = 7 * 24 * time.Hour
	duplicateWindow = 2 * time.Minute
)

type Publisher struct {
	repository Repository
	url        string
	logger     *slog.Logger

	mu sync.Mutex
	nc *nats.Conn
	js nats.JetStreamContext
}

func NewPublisher(repository Repository, url string, logger *slog.Logger) *Publisher {
	return &Publisher{repository: repository, url: url, logger: logger}
}

func (publisher *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	defer publisher.close()

	for {
		if err := publisher.publishPending(ctx); err != nil && !errors.Is(err, context.Canceled) {
			publisher.logger.Warn("unable to publish pending outbox events", "error", err)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
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
				return err
			}
			return publishErr
		}
		if err := publisher.repository.MarkPublished(ctx, event.EventID); err != nil {
			return err
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

	if _, err := publisher.js.StreamInfo(streamName); err == nil {
		return nil
	}

	_, err := publisher.js.AddStream(&nats.StreamConfig{
		Name:       streamName,
		Subjects:   []string{"tickets.>"},
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
