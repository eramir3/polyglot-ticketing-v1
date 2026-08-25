import { HttpStatus, ValidationError } from '@nestjs/common';
import { ApiError, ErrorItem } from './api-error';
import { ErrorCode, toPublicErrorCode } from './error-code';

export function createValidationApiError(
  validationErrors: ValidationError[],
): ApiError {
  return new ApiError(
    HttpStatus.BAD_REQUEST,
    validationErrors.map(toErrorItem),
  );
}

function toErrorItem(validationError: ValidationError): ErrorItem {
  const isUnexpectedField =
    validationError.constraints !== undefined &&
    'whitelistValidation' in validationError.constraints;

  return {
    message: isUnexpectedField
      ? 'Unexpected field.'
      : getValidationMessage(validationError),
    code: isUnexpectedField
      ? toPublicErrorCode(ErrorCode.INVALID_ARGUMENT)
      : getValidationCode(validationError.property),
    field: validationError.property,
  };
}

function getValidationMessage(validationError: ValidationError): string {
  const messages = Object.values(validationError.constraints ?? {});
  return messages[0] ?? 'Request data is invalid.';
}

function getValidationCode(field: string): string {
  switch (field) {
    case 'name':
      return toPublicErrorCode(ErrorCode.INVALID_NAME);
    case 'email':
      return toPublicErrorCode(ErrorCode.INVALID_EMAIL);
    case 'password':
      return toPublicErrorCode(ErrorCode.INVALID_PASSWORD);
    case 'title':
      return toPublicErrorCode(ErrorCode.INVALID_TITLE);
    case 'price':
      return toPublicErrorCode(ErrorCode.INVALID_PRICE);
    default:
      return toPublicErrorCode(ErrorCode.INVALID_ARGUMENT);
  }
}
