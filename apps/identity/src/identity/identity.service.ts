import { status } from '@grpc/grpc-js';
import { Inject, Injectable } from '@nestjs/common';
import { IDENTITY_AUTH_CONTEXT } from '../auth/auth.constants';
import { IdentityAuthContext } from '../auth/auth.factory';
import { ErrorItem, StructuredGrpcError } from '../errors/grpc-error';
import { SignUpRequest, SignUpResponse } from './identity.types';

@Injectable()
export class IdentityService {
  constructor(
    @Inject(IDENTITY_AUTH_CONTEXT)
    private readonly identityAuthContext: IdentityAuthContext,
  ) {}

  async signUp(request: SignUpRequest): Promise<SignUpResponse> {
    const validationErrors = validateSignUpRequest(request);
    if (validationErrors.length > 0) {
      throw new StructuredGrpcError(status.INVALID_ARGUMENT, validationErrors);
    }

    try {
      const response = await this.identityAuthContext.auth.api.signUpEmail({
        body: request,
      });

      return {
        email: response.user.email,
        userId: response.user.id,
      };
    } catch (error: unknown) {
      throw new StructuredGrpcError(getGrpcStatusCode(error), [
        getErrorItem(error),
      ]);
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

function getErrorItem(error: unknown): ErrorItem {
  const errorCode = getExternalErrorCode(error);

  if (errorCode === 'INVALID_EMAIL') {
    return {
      code: 'INVALID_EMAIL',
      field: 'email',
      message: 'Email must be valid.',
    };
  }

  if (
    errorCode === 'INVALID_PASSWORD' ||
    errorCode === 'PASSWORD_TOO_SHORT' ||
    errorCode === 'PASSWORD_TOO_LONG'
  ) {
    return {
      code: 'INVALID_PASSWORD',
      field: 'password',
      message: 'Password must be between 8 and 128 characters.',
    };
  }

  if (getHttpStatusCode(error) === 409) {
    return {
      code: 'ALREADY_EXISTS',
      field: 'email',
      message: 'A user with this email already exists.',
    };
  }

  if (getHttpStatusCode(error) !== undefined) {
    return {
      code: 'INVALID_ARGUMENT',
      message: 'Signup data is invalid.',
    };
  }

  return {
    code: 'INTERNAL_ERROR',
    message: 'Unable to complete signup.',
  };
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

function validateSignUpRequest(request: SignUpRequest): ErrorItem[] {
  const errors: ErrorItem[] = [];

  if (!isNonBlankString(request.name)) {
    errors.push({
      code: 'INVALID_NAME',
      field: 'name',
      message: 'Name is required.',
    });
  }

  if (!isValidEmail(request.email)) {
    errors.push({
      code: 'INVALID_EMAIL',
      field: 'email',
      message: 'Email must be valid.',
    });
  }

  if (
    typeof request.password !== 'string' ||
    request.password.length < 8 ||
    request.password.length > 128
  ) {
    errors.push({
      code: 'INVALID_PASSWORD',
      field: 'password',
      message: 'Password must be between 8 and 128 characters.',
    });
  }

  return errors;
}

function isNonBlankString(value: unknown): value is string {
  return typeof value === 'string' && value.trim().length > 0;
}

function isValidEmail(value: unknown): value is string {
  return typeof value === 'string' && /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}
