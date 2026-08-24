import { HttpStatus, ValidationError } from '@nestjs/common';
import { ApiError, ErrorItem } from './api-error';

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
      ? 'INVALID_ARGUMENT'
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
      return 'INVALID_NAME';
    case 'email':
      return 'INVALID_EMAIL';
    case 'password':
      return 'INVALID_PASSWORD';
    case 'title':
      return 'INVALID_TITLE';
    case 'price':
      return 'INVALID_PRICE';
    default:
      return 'INVALID_ARGUMENT';
  }
}
