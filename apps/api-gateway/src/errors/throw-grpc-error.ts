import { status } from '@grpc/grpc-js';
import { HttpStatus } from '@nestjs/common';
import { ApiError, ErrorItem } from './api-error';
import { ErrorCode, toPublicErrorCode } from './error-code';
import {
  getGrpcErrorResponse,
  httpStatusFromGrpcCode,
  isGrpcStatusError,
} from './grpc-error';

export function throwGatewayGrpcError(
  error: unknown,
  invalidArgumentError?: ErrorItem,
): never {
  if (isGrpcStatusError(error)) {
    const errorResponse = getGrpcErrorResponse(error);
    if (errorResponse) {
      throw new ApiError(
        httpStatusFromGrpcCode(error.code),
        errorResponse.errors,
      );
    }

    if (error.code === status.INVALID_ARGUMENT && invalidArgumentError) {
      throw new ApiError(HttpStatus.BAD_REQUEST, [invalidArgumentError]);
    }
  }

  throw new ApiError(HttpStatus.SERVICE_UNAVAILABLE, [
    {
      code: toPublicErrorCode(ErrorCode.SERVICE_UNAVAILABLE),
      message: 'A required service is unavailable.',
    },
  ]);
}
