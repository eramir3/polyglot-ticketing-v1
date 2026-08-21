import { status } from '@grpc/grpc-js';
import { Inject, Injectable } from '@nestjs/common';
import { IDENTITY_AUTH_CONTEXT } from '../auth/auth.constants';
import { IdentityAuthContext } from '../auth/auth.factory';
import { StructuredGrpcError } from '../errors/grpc-error';
import {
  SignUpRequest,
  SignUpResponse,
  VerifyEmailRequest,
  VerifyEmailResponse,
} from './identity.types';
import { mapBetterAuthError } from './mappers/better-auth-error.mapper';
import { mapEmailVerificationError } from './mappers/email-verification-error.mapper';
import { createSignupValidationInternalError } from './mappers/signup-errors';
import { validateSignUpRequest } from './signup-request.validator';

@Injectable()
export class IdentityService {
  constructor(
    @Inject(IDENTITY_AUTH_CONTEXT)
    private readonly identityAuthContext: IdentityAuthContext,
  ) {}

  async signUp(request: SignUpRequest): Promise<SignUpResponse> {
    const validationResult = validateSignUpRequest(request);
    if (validationResult.kind === 'invalid') {
      throw new StructuredGrpcError(
        status.INVALID_ARGUMENT,
        validationResult.errors,
      );
    }

    if (validationResult.kind === 'error') {
      throw new StructuredGrpcError(status.INTERNAL, [
        createSignupValidationInternalError(),
      ]);
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
      throw mapBetterAuthError(error);
    }
  }

  async verifyEmail(
    request: VerifyEmailRequest,
  ): Promise<VerifyEmailResponse> {
    try {
      await this.identityAuthContext.auth.api.verifyEmail({
        query: { token: request.token },
      });

      return { verified: true };
    } catch (error: unknown) {
      throw mapEmailVerificationError(error);
    }
  }
}
