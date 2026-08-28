package payment

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
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

func TestParseRandomFailures(t *testing.T) {
	testCases := []struct {
		value    string
		expected bool
		invalid  bool
	}{
		{value: "", expected: false},
		{value: "false", expected: false},
		{value: "TRUE", expected: true},
		{value: "random", invalid: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.value, func(t *testing.T) {
			randomFailures, err := ParseRandomFailures(testCase.value)
			if testCase.invalid {
				if !errors.Is(err, ErrInvalidRandomFailures) {
					t.Fatalf("expected invalid random failures error, got %v", err)
				}
				return
			}
			if err != nil || randomFailures != testCase.expected {
				t.Fatalf("parse random failures: value=%t err=%v", randomFailures, err)
			}
		})
	}
}

func TestProcessorResolvesUsingConfiguredOutcome(t *testing.T) {
	for _, outcome := range []ProcessorOutcome{ProcessorOutcomeSuccess, ProcessorOutcomeFailure} {
		t.Run(string(outcome), func(t *testing.T) {
			repository := &fakeSettlementRepository{payments: []Payment{{ID: "payment-1", OrderID: "order-1"}}}
			processor := NewProcessor(repository, outcome, false, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))

			_, resolved, err := processor.ProcessOnce(context.Background())

			if err != nil || !resolved || repository.outcome != outcome {
				t.Fatalf("process payment: resolved=%t outcome=%q err=%v", resolved, repository.outcome, err)
			}
		})
	}
}

func TestProcessorRandomFailureMode(t *testing.T) {
	testCases := []struct {
		name            string
		randomFloat64   float64
		expectedOutcome ProcessorOutcome
	}{
		{name: "below failure threshold", randomFloat64: 0.099, expectedOutcome: ProcessorOutcomeFailure},
		{name: "at failure threshold", randomFloat64: 0.10, expectedOutcome: ProcessorOutcomeSuccess},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repository := &fakeSettlementRepository{payments: []Payment{{ID: "payment-1", OrderID: "order-1"}}}
			processor := newProcessor(
				repository,
				ProcessorOutcomeFailure,
				true,
				func() float64 { return testCase.randomFloat64 },
				slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
			)

			_, resolved, err := processor.ProcessOnce(context.Background())

			if err != nil || !resolved || repository.outcome != testCase.expectedOutcome {
				t.Fatalf("process payment: resolved=%t outcome=%q err=%v", resolved, repository.outcome, err)
			}
		})
	}
}

func TestProcessorLogsFailedPayment(t *testing.T) {
	var logs bytes.Buffer
	processor := NewProcessor(
		&fakeSettlementRepository{payments: []Payment{{
			ID:      "payment-1",
			OrderID: "order-1",
			Status:  StatusFailed,
		}}},
		ProcessorOutcomeFailure,
		false,
		slog.New(slog.NewTextHandler(&logs, nil)),
	)

	processor.processPending(context.Background())

	output := logs.String()
	for _, expected := range []string{
		"level=WARN",
		"msg=\"payment failed\"",
		"operation=resolve_payment",
		"payment_id=payment-1",
		"order_id=order-1",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("expected log output to contain %q, got %q", expected, output)
		}
	}
}

type fakeSettlementRepository struct {
	err      error
	outcome  ProcessorOutcome
	payments []Payment
}

func (repository *fakeSettlementRepository) ResolveNextPending(_ context.Context, outcome ProcessorOutcome) (Payment, bool, error) {
	repository.outcome = outcome
	if repository.err != nil {
		return Payment{}, false, repository.err
	}
	if len(repository.payments) == 0 {
		return Payment{}, false, nil
	}
	resolved := repository.payments[0]
	repository.payments = repository.payments[1:]
	return resolved, true, nil
}
