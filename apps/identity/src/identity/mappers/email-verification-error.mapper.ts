import { status } from '@grpc/grpc-js';
import { ErrorCode, toPublicErrorCode } from '../../errors/error-code';
import { StructuredGrpcError } from '../../errors/grpc-error';

export function mapEmailVerificationError(error: unknown): StructuredGrpcError {
  if (hasVerificationErrorCode(error)) {
    return new StructuredGrpcError(status.UNAUTHENTICATED, [
      {
        code: toPublicErrorCode(ErrorCode.INVALID_VERIFICATION_TOKEN),
        message: 'The email verification link is invalid or has expired.',
      },
    ]);
  }

  return new StructuredGrpcError(status.INTERNAL, [
    {
      code: toPublicErrorCode(ErrorCode.INTERNAL_ERROR),
      message: 'Unable to verify email.',
    },
  ]);
}

function hasVerificationErrorCode(error: unknown): boolean {
  const code = getExternalErrorCode(error);
  return code === 'INVALID_TOKEN' || code === 'TOKEN_EXPIRED';
}

function getExternalErrorCode(error: unknown): string | undefined {
  if (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    typeof error.code === 'string'
  ) {
    return error.code;
  }

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
