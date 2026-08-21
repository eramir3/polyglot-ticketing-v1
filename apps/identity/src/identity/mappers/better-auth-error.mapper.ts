import { status } from '@grpc/grpc-js';
import { StructuredGrpcError } from '../../errors/grpc-error';
import {
  createEmailAlreadyExistsError,
  createInvalidArgumentError,
  createInvalidEmailError,
  createInvalidPasswordError,
  createSignupInternalError,
} from './signup-errors';

export function mapBetterAuthError(error: unknown): StructuredGrpcError {
  return new StructuredGrpcError(getGrpcStatusCode(error), [
    getErrorItem(error),
  ]);
}

function getGrpcStatusCode(error: unknown): status {
  const statusCode = getHttpStatusCode(error);

  if (statusCode !== undefined) {
    if (statusCode === 409) {
      return status.ALREADY_EXISTS;
    }

    if (statusCode >= 400 && statusCode < 500) {
      return status.INVALID_ARGUMENT;
    }
  }

  return status.INTERNAL;
}

function getErrorItem(error: unknown) {
  const errorCode = getExternalErrorCode(error);

  if (errorCode === 'INVALID_EMAIL') {
    return createInvalidEmailError();
  }

  if (
    errorCode === 'INVALID_PASSWORD' ||
    errorCode === 'PASSWORD_TOO_SHORT' ||
    errorCode === 'PASSWORD_TOO_LONG'
  ) {
    return createInvalidPasswordError();
  }

  if (getHttpStatusCode(error) === 409) {
    return createEmailAlreadyExistsError();
  }

  if (getHttpStatusCode(error) !== undefined) {
    return createInvalidArgumentError();
  }

  return createSignupInternalError();
}

function getHttpStatusCode(error: unknown): number | undefined {
  if (hasNumericStatusCode(error)) {
    return error.statusCode;
  }

  if (hasNumericStatus(error)) {
    return error.status;
  }

  return undefined;
}

function hasNumericStatusCode(error: unknown): error is { statusCode: number } {
  return (
    typeof error === 'object' &&
    error !== null &&
    'statusCode' in error &&
    typeof error.statusCode === 'number'
  );
}

function hasNumericStatus(error: unknown): error is { status: number } {
  return (
    typeof error === 'object' &&
    error !== null &&
    'status' in error &&
    typeof error.status === 'number'
  );
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

  return undefined;
}
