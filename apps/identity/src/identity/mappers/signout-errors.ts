import { ErrorItem } from '../../errors/grpc-error';

export function createSignOutInternalError(): ErrorItem {
  return {
    code: 'INTERNAL_ERROR',
    message: 'Unable to complete signout.',
  };
}
