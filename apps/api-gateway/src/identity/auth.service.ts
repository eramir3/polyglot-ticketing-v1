import { IncomingHttpHeaders } from 'node:http';
import { Inject, Injectable, OnModuleInit } from '@nestjs/common';
import type { ClientGrpc } from '@nestjs/microservices';
import { firstValueFrom } from 'rxjs';
import { throwGatewayGrpcError } from '../errors/throw-grpc-error';
import { IDENTITY_GRPC_CLIENT } from './identity.constants';
import { createAuthRequestMetadata } from './metadata/auth-request-metadata';
import { createSessionRequestMetadata } from './metadata/session-request-metadata';
import {
  CurrentUserRequest,
  CurrentUserResponse,
  IdentityCurrentUserResponse,
  IdentitySignInResponse,
  IdentityGrpcService,
  SignInRequest,
  SignOutRequest,
  SignUpRequest,
  SignUpResponse,
  VerifyEmailRequest,
  VerifyEmailResponse,
} from './identity.types';

@Injectable()
export class AuthService implements OnModuleInit {
  private identityService!: IdentityGrpcService;

  constructor(
    @Inject(IDENTITY_GRPC_CLIENT) private readonly identityClient: ClientGrpc,
  ) {}

  onModuleInit(): void {
    this.identityService =
      this.identityClient.getService<IdentityGrpcService>('IdentityService');
  }

  async signUp(request: SignUpRequest): Promise<SignUpResponse> {
    try {
      return await firstValueFrom(this.identityService.signUp(request));
    } catch (error: unknown) {
      throwGatewayGrpcError(error, {
        code: 'INVALID_ARGUMENT',
        message: 'Signup data is invalid.',
      });
    }
  }

  async signIn(
    request: SignInRequest,
    headers: IncomingHttpHeaders,
  ): Promise<IdentitySignInResponse> {
    try {
      return await firstValueFrom(
        this.identityService.signIn(
          request,
          createAuthRequestMetadata(headers),
        ),
      );
    } catch (error: unknown) {
      throwGatewayGrpcError(error, {
        code: 'INVALID_ARGUMENT',
        message: 'Signin data is invalid.',
      });
    }
  }

  async signOut(headers: IncomingHttpHeaders): Promise<void> {
    try {
      await firstValueFrom(
        this.identityService.signOut(
          {} satisfies SignOutRequest,
          createSessionRequestMetadata(headers),
        ),
      );
    } catch (error: unknown) {
      throwGatewayGrpcError(error);
    }
  }

  async currentUser(
    headers: IncomingHttpHeaders,
  ): Promise<CurrentUserResponse> {
    try {
      const response = await firstValueFrom(
        this.identityService.currentUser(
          {} satisfies CurrentUserRequest,
          createSessionRequestMetadata(headers),
        ),
      );

      return toCurrentUserResponse(response);
    } catch (error: unknown) {
      throwGatewayGrpcError(error);
    }
  }

  async verifyEmail(request: VerifyEmailRequest): Promise<VerifyEmailResponse> {
    try {
      return await firstValueFrom(this.identityService.verifyEmail(request));
    } catch (error: unknown) {
      throwGatewayGrpcError(error);
    }
  }
}

function toCurrentUserResponse(
  response: IdentityCurrentUserResponse,
): CurrentUserResponse {
  return {
    user: {
      email: response.email,
      emailVerified: response.emailVerified,
      id: response.userId,
      name: response.name,
    },
  };
}
