import { status } from '@grpc/grpc-js';
import { HttpStatus, Inject, Injectable, OnModuleInit } from '@nestjs/common';
import type { ClientGrpc } from '@nestjs/microservices';
import { firstValueFrom } from 'rxjs';
import { ApiError } from '../errors/api-error';
import {
  getGrpcErrorResponse,
  httpStatusFromGrpcCode,
  isGrpcStatusError,
} from '../errors/grpc-error';
import {
  IdentityGrpcService,
  SignUpRequest,
  SignUpResponse,
} from './identity.types';
import { IDENTITY_GRPC_CLIENT } from './identity.constants';

@Injectable()
export class SignupService implements OnModuleInit {
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
      if (isGrpcStatusError(error)) {
        const errorResponse = getGrpcErrorResponse(error);
        if (errorResponse) {
          throw new ApiError(
            httpStatusFromGrpcCode(error.code),
            errorResponse.errors,
          );
        }

        if (error.code === status.INVALID_ARGUMENT) {
          throw new ApiError(HttpStatus.BAD_REQUEST, [
            {
              code: 'INVALID_ARGUMENT',
              message: 'Signup data is invalid.',
            },
          ]);
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
}
