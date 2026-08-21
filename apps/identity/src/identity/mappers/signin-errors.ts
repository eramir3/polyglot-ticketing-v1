import { ErrorItem } from '../../errors/grpc-error';

export function createEmailNotVerifiedError(): ErrorItem {
  return {
    code: 'EMAIL_NOT_VERIFIED',
    message: 'Verify your email before signing in.',
  };
}

export function createInvalidCredentialsError(): ErrorItem {
  return {
    code: 'INVALID_CREDENTIALS',
    message: 'Email or password is incorrect.',
  };
}

export function createSigninInternalError(): ErrorItem {
  return {
    code: 'INTERNAL_ERROR',
    message: 'Unable to complete signin.',
  };
}

export function createSigninValidationInternalError(): ErrorItem {
  return {
    code: 'INTERNAL_ERROR',
    message: 'Unable to validate signin data.',
  };
}
