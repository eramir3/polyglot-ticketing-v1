import { status } from '@grpc/grpc-js';
import { Inject, Injectable } from '@nestjs/common';
import { RpcException } from '@nestjs/microservices';
import { IDENTITY_AUTH_CONTEXT } from '../auth/auth.constants';
import { IdentityAuthContext } from '../auth/auth.factory';
import { SignUpRequest, SignUpResponse } from './identity.types';

@Injectable()
export class IdentityService {
  constructor(
    @Inject(IDENTITY_AUTH_CONTEXT)
    private readonly identityAuthContext: IdentityAuthContext
  ) {}

  async signUp(request: SignUpRequest): Promise<SignUpResponse> {
    try {
      const response = await this.identityAuthContext.auth.api.signUpEmail({
        body: request,
      });

      return {
        email: response.user.email,
        userId: response.user.id,
      };
    } catch (error: unknown) {
      throw new RpcException({
        code: getGrpcStatusCode(error),
        details: getErrorMessage(error),
      });
    }
  }
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

function getErrorMessage(error: unknown): string {
  if (hasMessage(error)) {
    return error.message;
  }

  return 'Identity signup failed.';
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

function hasMessage(error: unknown): error is { message: string } {
  return (
    typeof error === 'object' &&
    error !== null &&
    'message' in error &&
    typeof error.message === 'string'
  );
}
