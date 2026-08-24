import { status } from '@grpc/grpc-js';
import { StructuredGrpcError } from '../../errors/grpc-error';
import { createSignOutInternalError } from './signout-errors';

export function mapBetterAuthSignOutError(): StructuredGrpcError {
  return new StructuredGrpcError(status.INTERNAL, [
    createSignOutInternalError(),
  ]);
}
