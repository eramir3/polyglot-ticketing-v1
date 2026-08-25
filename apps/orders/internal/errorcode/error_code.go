package errorcode

import (
	"strings"

	commonv1 "polyglot-ticketing-v1/protogen/go/common/v1"
)

const enumPrefix = "ERROR_CODE_"

func String(code commonv1.ErrorCode) string {
	name := code.String()
	if code == commonv1.ErrorCode_ERROR_CODE_UNSPECIFIED || !strings.HasPrefix(name, enumPrefix) {
		name = commonv1.ErrorCode_ERROR_CODE_INTERNAL_ERROR.String()
	}

	return strings.TrimPrefix(name, enumPrefix)
}
