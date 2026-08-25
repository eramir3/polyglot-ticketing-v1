import { ErrorItem } from '../../errors/grpc-error';
import { ErrorCode, toPublicErrorCode } from '../../errors/error-code';

export function createSignOutInternalError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INTERNAL_ERROR),
    message: 'Unable to complete signout.',
  };
}
