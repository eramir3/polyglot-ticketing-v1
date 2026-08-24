import { status } from '@grpc/grpc-js';
import { IncomingHttpHeaders } from 'node:http';
import { HttpStatus, Inject, Injectable, OnModuleInit } from '@nestjs/common';
import type { ClientGrpc } from '@nestjs/microservices';
import { firstValueFrom } from 'rxjs';
import { ApiError, ErrorItem } from '../errors/api-error';
import {
  getGrpcErrorResponse,
  httpStatusFromGrpcCode,
  isGrpcStatusError,
} from '../errors/grpc-error';
import { IDENTITY_GRPC_CLIENT } from './identity.constants';
import { createAuthRequestMetadata } from './metadata/auth-request-metadata';
import { createSignOutRequestMetadata } from './metadata/signout-request-metadata';
import {
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
      this.throwGrpcError(error, {
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
      this.throwGrpcError(error, {
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
          createSignOutRequestMetadata(headers),
        ),
      );
    } catch (error: unknown) {
      this.throwGrpcError(error);
    }
  }

  async verifyEmail(request: VerifyEmailRequest): Promise<VerifyEmailResponse> {
    try {
      return await firstValueFrom(this.identityService.verifyEmail(request));
    } catch (error: unknown) {
      this.throwGrpcError(error);
    }
  }

  private throwGrpcError(
    error: unknown,
    invalidArgumentError?: ErrorItem,
  ): never {
    if (isGrpcStatusError(error)) {
      const errorResponse = getGrpcErrorResponse(error);
      if (errorResponse) {
        throw new ApiError(
          httpStatusFromGrpcCode(error.code),
          errorResponse.errors,
        );
      }

      if (error.code === status.INVALID_ARGUMENT && invalidArgumentError) {
        throw new ApiError(HttpStatus.BAD_REQUEST, [invalidArgumentError]);
      }
    }

    throw new ApiError(HttpStatus.SERVICE_UNAVAILABLE, [
      {
        code: 'SERVICE_UNAVAILABLE',
        message: 'A required service is unavailable.',
      },
    ]);
  }
}
