package payment

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

const processorPollInterval = time.Second

// Processor resolves pending payments using the configured local simulation.
// Each resolution and its result event are committed in one transaction by the
// repository, so a retry cannot publish a second outcome.
type Processor struct {
	logger  *slog.Logger
	outcome ProcessorOutcome
	repo    SettlementRepository
}

func NewProcessor(repo SettlementRepository, outcome ProcessorOutcome, logger *slog.Logger) *Processor {
	return &Processor{logger: logger, outcome: outcome, repo: repo}
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

func (processor *Processor) ProcessOnce(ctx context.Context) (bool, error) {
	return processor.repo.ResolveNextPending(ctx, processor.outcome)
}

func (processor *Processor) processPending(ctx context.Context) {
	for ctx.Err() == nil {
		resolved, err := processor.ProcessOnce(ctx)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				processor.logger.Warn("unable to resolve pending payment", "error", err)
			}
			return
		}
		if !resolved {
			return
		}
	}
}
