package payment

import (
	"context"
	"errors"
	"log/slog"
	"math/rand/v2"
	"time"

	"polyglot-ticketing-v1/internal/observability"
)

const (
	processorPollInterval    = time.Second
	randomFailureProbability = 0.10
)

// Processor resolves pending payments using the configured local simulation.
// Each resolution and its result event are committed in one transaction by the
// repository, so a retry cannot publish a second outcome.
type Processor struct {
	logger         *slog.Logger
	outcome        ProcessorOutcome
	randomFailures bool
	randomFloat64  func() float64
	repo           SettlementRepository
	metrics        *observability.Metrics
}

func NewProcessor(repo SettlementRepository, outcome ProcessorOutcome, randomFailures bool, logger *slog.Logger, metrics ...*observability.Metrics) *Processor {
	processor := newProcessor(repo, outcome, randomFailures, rand.Float64, logger)
	if len(metrics) > 0 {
		processor.metrics = metrics[0]
	}
	return processor
}

func newProcessor(
	repo SettlementRepository,
	outcome ProcessorOutcome,
	randomFailures bool,
	randomFloat64 func() float64,
	logger *slog.Logger,
) *Processor {
	return &Processor{
		logger:         logger,
		outcome:        outcome,
		randomFailures: randomFailures,
		randomFloat64:  randomFloat64,
		repo:           repo,
	}
}

func (processor *Processor) Run(ctx context.Context) {
	ticker := time.NewTicker(processorPollInterval)
	defer ticker.Stop()

	for {
		processor.processPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (processor *Processor) ProcessOnce(ctx context.Context) (Payment, bool, error) {
	return processor.repo.ResolveNextPending(ctx, processor.nextOutcome())
}

func (processor *Processor) nextOutcome() ProcessorOutcome {
	if processor.randomFailures {
		if processor.randomFloat64() < randomFailureProbability {
			return ProcessorOutcomeFailure
		}
		return ProcessorOutcomeSuccess
	}
	return processor.outcome
}

func (processor *Processor) processPending(ctx context.Context) {
	for ctx.Err() == nil {
		started := time.Now()
		payment, resolved, err := processor.ProcessOnce(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				processor.logger.Warn("unable to resolve pending payment", "error", err)
				processor.observe("failure", started)
			}
			return
		}
		if !resolved {
			return
		}
		outcome := "succeeded"
		if payment.Status == StatusFailed {
			outcome = "failed"
		}
		processor.observe(outcome, started)
		if payment.Status == StatusFailed {
			processor.logger.Warn(
				"payment failed",
				"operation", "resolve_payment",
				"payment_id", payment.ID,
				"order_id", payment.OrderID,
			)
		}
	}
}

func (processor *Processor) observe(outcome string, started time.Time) {
	if processor.metrics != nil {
		processor.metrics.ObserveBackground("payment_processor", "resolve_payment", outcome, started)
	}
}
