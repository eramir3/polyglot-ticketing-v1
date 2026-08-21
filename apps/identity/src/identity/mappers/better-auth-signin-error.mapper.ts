import { status } from '@grpc/grpc-js';
import { StructuredGrpcError } from '../../errors/grpc-error';
import {
  createEmailNotVerifiedError,
  createInvalidCredentialsError,
  createSigninInternalError,
} from './signin-errors';

export function mapBetterAuthSignInError(error: unknown): StructuredGrpcError {
  const errorCode = getExternalErrorCode(error);

  if (errorCode === 'EMAIL_NOT_VERIFIED') {
    return new StructuredGrpcError(status.UNAUTHENTICATED, [
      createEmailNotVerifiedError(),
    ]);
  }

  if (errorCode === 'INVALID_EMAIL_OR_PASSWORD') {
    return new StructuredGrpcError(status.UNAUTHENTICATED, [
      createInvalidCredentialsError(),
    ]);
  }

  return new StructuredGrpcError(status.INTERNAL, [
    createSigninInternalError(),
  ]);
}

function getExternalErrorCode(error: unknown): string | undefined {
  if (
    typeof error === 'object' &&
    error !== null &&
    'body' in error &&
    typeof error.body === 'object' &&
    error.body !== null &&
    'code' in error.body &&
    typeof error.body.code === 'string'
  ) {
    return error.body.code;
  }

  if (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    typeof error.code === 'string'
  ) {
    return error.code;
  }

  return undefined;
}
