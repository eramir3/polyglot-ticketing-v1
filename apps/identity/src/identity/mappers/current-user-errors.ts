import { ErrorItem } from '../../errors/grpc-error';

export function createCurrentUserUnauthenticatedError(): ErrorItem {
  return {
    code: 'UNAUTHENTICATED',
    message: 'Authentication is required.',
  };
}

export function createCurrentUserInternalError(): ErrorItem {
  return {
    code: 'INTERNAL_ERROR',
    message: 'Unable to get the current user.',
  };
}
