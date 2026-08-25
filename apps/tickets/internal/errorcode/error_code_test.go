package errorcode

import (
	"testing"

	commonv1 "polyglot-ticketing-v1/protogen/go/common/v1"
)

func TestStringReturnsPublicCodeForEveryDefinedErrorCode(t *testing.T) {
	for number, name := range commonv1.ErrorCode_name {
		code := commonv1.ErrorCode(number)
		if code == commonv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			continue
		}

		want := name[len("ERROR_CODE_"):]
		if got := String(code); got != want {
			t.Fatalf("String(%v) = %q, want %q", code, got, want)
		}
	}
}

func TestStringFallsBackToInternalError(t *testing.T) {
	for _, code := range []commonv1.ErrorCode{
		commonv1.ErrorCode_ERROR_CODE_UNSPECIFIED,
		commonv1.ErrorCode(999),
	} {
		if got := String(code); got != "INTERNAL_ERROR" {
			t.Fatalf("String(%v) = %q, want INTERNAL_ERROR", code, got)
		}
	}
}
