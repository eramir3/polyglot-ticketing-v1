package payment

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

func TestParseProcessorOutcome(t *testing.T) {
	testCases := []struct {
		value    string
		expected ProcessorOutcome
		invalid  bool
	}{
		{value: "", expected: ProcessorOutcomeSuccess},
		{value: "SUCCESS", expected: ProcessorOutcomeSuccess},
		{value: "failure", expected: ProcessorOutcomeFailure},
		{value: "unknown", invalid: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.value, func(t *testing.T) {
			outcome, err := ParseProcessorOutcome(testCase.value)
			if testCase.invalid {
				if !errors.Is(err, ErrInvalidOutcome) {
					t.Fatalf("expected invalid outcome error, got %v", err)
				}
				return
			}
			if err != nil || outcome != testCase.expected {
				t.Fatalf("parse outcome: outcome=%q err=%v", outcome, err)
			}
		})
	}
}

func TestProcessorResolvesUsingConfiguredOutcome(t *testing.T) {
	repository := &fakeSettlementRepository{resolved: true}
	processor := NewProcessor(repository, ProcessorOutcomeFailure, slog.New(slog.NewTextHandler(io.Discard, nil)))

	resolved, err := processor.ProcessOnce(context.Background())

	if err != nil || !resolved || repository.outcome != ProcessorOutcomeFailure {
		t.Fatalf("process payment: resolved=%t outcome=%q err=%v", resolved, repository.outcome, err)
	}
}

type fakeSettlementRepository struct {
	err      error
	outcome  ProcessorOutcome
	resolved bool
}

func (repository *fakeSettlementRepository) ResolveNextPending(_ context.Context, outcome ProcessorOutcome) (bool, error) {
	repository.outcome = outcome
	return repository.resolved, repository.err
}
