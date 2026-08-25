import { ErrorItem } from '../../errors/grpc-error';
import { ErrorCode, toPublicErrorCode } from '../../errors/error-code';

export function createEmailNotVerifiedError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.EMAIL_NOT_VERIFIED),
    message: 'Verify your email before signing in.',
  };
}

export function createInvalidCredentialsError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INVALID_CREDENTIALS),
    message: 'Email or password is incorrect.',
  };
}

export function createSigninInternalError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INTERNAL_ERROR),
    message: 'Unable to complete signin.',
  };
}

export function createSigninValidationInternalError(): ErrorItem {
  return {
    code: toPublicErrorCode(ErrorCode.INTERNAL_ERROR),
    message: 'Unable to validate signin data.',
  };
}
