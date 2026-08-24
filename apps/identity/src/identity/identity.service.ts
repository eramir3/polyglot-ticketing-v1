import { Metadata, status } from '@grpc/grpc-js';
import { Inject, Injectable } from '@nestjs/common';
import { IDENTITY_AUTH_CONTEXT } from '../auth/auth.constants';
import { IdentityAuthContext } from '../auth/auth.factory';
import { StructuredGrpcError } from '../errors/grpc-error';
import {
  CurrentUserRequest,
  CurrentUserResponse,
  SignInRequest,
  SignInResponse,
  SignOutRequest,
  SignOutResponse,
  SignUpRequest,
  SignUpResponse,
  VerifyEmailRequest,
  VerifyEmailResponse,
} from './identity.types';
import { mapBetterAuthError } from './mappers/better-auth-error.mapper';
import { mapBetterAuthSignInError } from './mappers/better-auth-signin-error.mapper';
import { mapBetterAuthSignOutError } from './mappers/better-auth-signout-error.mapper';
import { mapEmailVerificationError } from './mappers/email-verification-error.mapper';
import { createSigninValidationInternalError } from './mappers/signin-errors';
import { createSignupValidationInternalError } from './mappers/signup-errors';
import { createAuthRequestHeaders } from './headers/signin-request-headers';
import { extractSessionCookieValue } from './session-cookie';
import { createSessionRequestHeaders } from './headers/session-request-headers';
import { validateSignInRequest } from './validators/signin-request.validator';
import { validateSignUpRequest } from './validators/signup-request.validator';
import {
  createCurrentUserInternalError,
  createCurrentUserUnauthenticatedError,
} from './mappers/current-user-errors';

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

  async signIn(
    request: SignInRequest,
    metadata: Metadata,
  ): Promise<SignInResponse> {
    const validationResult = validateSignInRequest(request);
    if (validationResult.kind === 'invalid') {
      throw new StructuredGrpcError(
        status.INVALID_ARGUMENT,
        validationResult.errors,
      );
    }

    if (validationResult.kind === 'error') {
      throw new StructuredGrpcError(status.INTERNAL, [
        createSigninValidationInternalError(),
      ]);
    }

    try {
      const signInResult = await this.identityAuthContext.auth.api.signInEmail({
        body: request,
        headers: createAuthRequestHeaders(metadata),
        returnHeaders: true,
      });
      const sessionCookieValue = extractSessionCookieValue(
        signInResult.headers,
      );
      const response = signInResult.response;

      if (!sessionCookieValue) {
        throw new Error(
          'Better Auth did not return the created session cookie.',
        );
      }

      const sessionExpiresAt = await this.identityAuthContext.getSessionExpiry(
        response.token,
        response.user.id,
      );

      if (!sessionExpiresAt) {
        throw new Error('Better Auth did not return the created session.');
      }

      return {
        email: response.user.email,
        emailVerified: response.user.emailVerified,
        name: response.user.name,
        sessionExpiresAt: toTimestamp(sessionExpiresAt),
        sessionToken: sessionCookieValue,
        userId: response.user.id,
      };
    } catch (error: unknown) {
      throw mapBetterAuthSignInError(error);
    }
  }

  async signOut(
    _request: SignOutRequest,
    metadata: Metadata,
  ): Promise<SignOutResponse> {
    try {
      await this.identityAuthContext.auth.api.signOut({
        headers: createSessionRequestHeaders(metadata),
      });

      return {};
    } catch {
      throw mapBetterAuthSignOutError();
    }
  }

  async currentUser(
    _request: CurrentUserRequest,
    metadata: Metadata,
  ): Promise<CurrentUserResponse> {
    let currentSession;

    try {
      currentSession = await this.identityAuthContext.auth.api.getSession({
        headers: createSessionRequestHeaders(metadata),
      });
    } catch {
      throw new StructuredGrpcError(status.INTERNAL, [
        createCurrentUserInternalError(),
      ]);
    }

    if (!currentSession) {
      throw new StructuredGrpcError(status.UNAUTHENTICATED, [
        createCurrentUserUnauthenticatedError(),
      ]);
    }

    return {
      email: currentSession.user.email,
      emailVerified: currentSession.user.emailVerified,
      name: currentSession.user.name,
      userId: currentSession.user.id,
    };
  }

  async verifyEmail(request: VerifyEmailRequest): Promise<VerifyEmailResponse> {
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

function toTimestamp(value: Date): { seconds: string; nanos: number } {
  const milliseconds = value.getTime();
  return {
    nanos: (milliseconds % 1_000) * 1_000_000,
    seconds: Math.floor(milliseconds / 1_000).toString(),
  };
}
