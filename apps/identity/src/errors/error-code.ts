import { ErrorCode } from '../../../../protogen/ts/common/v1/error_pb.js';

export { ErrorCode };

export function toPublicErrorCode(errorCode: ErrorCode): string {
  const code = ErrorCode[errorCode];

  if (errorCode === ErrorCode.UNSPECIFIED || typeof code !== 'string') {
    return ErrorCode[ErrorCode.INTERNAL_ERROR] as string;
  }

  return code;
}
