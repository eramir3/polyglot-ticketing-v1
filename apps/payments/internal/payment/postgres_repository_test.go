package payment

import (
	"errors"
	"testing"
)

func TestValidateOrderVersion(t *testing.T) {
	testCases := []struct {
		name            string
		hasStoredOrder  bool
		incomingVersion int64
		storedVersion   int64
		wantErr         error
	}{
		{name: "accepts initial version", incomingVersion: 0},
		{name: "rejects missing initial version", incomingVersion: 1, wantErr: ErrOrderEventVersionGap},
		{name: "accepts next version", hasStoredOrder: true, storedVersion: 0, incomingVersion: 1},
		{name: "accepts duplicate version", hasStoredOrder: true, storedVersion: 1, incomingVersion: 1},
		{name: "accepts stale version", hasStoredOrder: true, storedVersion: 2, incomingVersion: 1},
		{name: "rejects skipped version", hasStoredOrder: true, storedVersion: 1, incomingVersion: 3, wantErr: ErrOrderEventVersionGap},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateOrderVersion(testCase.hasStoredOrder, testCase.storedVersion, testCase.incomingVersion)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("expected error %v, got %v", testCase.wantErr, err)
			}
		})
	}
}
