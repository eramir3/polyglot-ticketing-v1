import { ErrorItem } from '../../errors/grpc-error';
import { ErrorCode, toPublicErrorCode } from '../../errors/error-code';

export function createCurrentUserUnauthenticatedError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.UNAUTHENTICATED),
    message: 'Authentication is required.',
  };
}

export function createCurrentUserInternalError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INTERNAL_ERROR),
    message: 'Unable to get the current user.',
  };
}
