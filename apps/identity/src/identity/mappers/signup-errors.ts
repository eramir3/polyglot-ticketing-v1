import { ErrorItem } from '../../errors/grpc-error';
import { ErrorCode, toPublicErrorCode } from '../../errors/error-code';

export function createInvalidNameError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INVALID_NAME),
    field: 'name',
    message: 'Name is required.',
  };
}

export function createInvalidEmailError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INVALID_EMAIL),
    field: 'email',
    message: 'Email must be valid.',
  };
}

export function createInvalidPasswordError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INVALID_PASSWORD),
    field: 'password',
    message: 'Password must be between 8 and 128 characters.',
  };
}

export function createInvalidArgumentError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INVALID_ARGUMENT),
    message: 'Signup data is invalid.',
  };
}

export function createEmailAlreadyExistsError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.ALREADY_EXISTS),
    field: 'email',
    message: 'A user with this email already exists.',
  };
}

export function createSignupValidationInternalError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INTERNAL_ERROR),
    message: 'Unable to validate signup data.',
  };
}

export function createSignupInternalError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INTERNAL_ERROR),
    message: 'Unable to complete signup.',
  };
}
