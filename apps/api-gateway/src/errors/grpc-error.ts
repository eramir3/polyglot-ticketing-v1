import { status } from '@grpc/grpc-js';
import { ErrorItem, ErrorResponse } from './api-error';

export interface GrpcStatusError {
  code: number;
  details?: string;
}

export function isGrpcStatusError(error: unknown): error is GrpcStatusError {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    typeof error.code === 'number'
  );
}

export function getGrpcErrorResponse(
  error: GrpcStatusError,
): ErrorResponse | undefined {
  if (typeof error.details !== 'string') {
    return undefined;
  }

  try {
    const parsed: unknown = JSON.parse(error.details);
    if (hasErrorItems(parsed)) {
      return { errors: parsed.errors };
    }
  } catch {
    return undefined;
  }

  return undefined;
}

export function httpStatusFromGrpcCode(code: number): number {
  switch (code) {
    case status.INVALID_ARGUMENT:
      return 400;
    case status.UNAUTHENTICATED:
      return 401;
    case status.PERMISSION_DENIED:
      return 403;
    case status.NOT_FOUND:
      return 404;
    case status.ALREADY_EXISTS:
      return 409;
    case status.RESOURCE_EXHAUSTED:
      return 429;
    case status.UNAVAILABLE:
      return 503;
    default:
      return 500;
  }
}

function hasErrorItems(value: unknown): value is { errors: ErrorItem[] } {
  return (
    typeof value === 'object' &&
    value !== null &&
    'errors' in value &&
    Array.isArray(value.errors) &&
    value.errors.length > 0 &&
    value.errors.every(isErrorItem)
  );
}

function isErrorItem(value: unknown): value is ErrorItem {
  return (
    typeof value === 'object' &&
    value !== null &&
    'message' in value &&
    typeof value.message === 'string' &&
    'code' in value &&
    typeof value.code === 'string' &&
    (!('field' in value) || typeof value.field === 'string')
  );
}
