import { Inject, Injectable, OnModuleInit } from '@nestjs/common';
import type { ClientGrpc } from '@nestjs/microservices';
import { firstValueFrom } from 'rxjs';
import { ApiError } from '../errors/api-error';
import {
  getGrpcErrorResponse,
  httpStatusFromGrpcCode,
  isGrpcStatusError,
} from '../errors/grpc-error';
import { IDENTITY_GRPC_CLIENT } from './identity.constants';
import {
  IdentityGrpcService,
  VerifyEmailRequest,
  VerifyEmailResponse,
} from './identity.types';

@Injectable()
export class VerifyEmailService implements OnModuleInit {
  private identityService!: IdentityGrpcService;

  constructor(
    @Inject(IDENTITY_GRPC_CLIENT) private readonly identityClient: ClientGrpc,
  ) {}

  onModuleInit(): void {
    this.identityService =
      this.identityClient.getService<IdentityGrpcService>('IdentityService');
  }

  async verifyEmail(
    request: VerifyEmailRequest,
  ): Promise<VerifyEmailResponse> {
    try {
      return await firstValueFrom(this.identityService.verifyEmail(request));
    } catch (error: unknown) {
      if (isGrpcStatusError(error)) {
        const errorResponse = getGrpcErrorResponse(error);
        if (errorResponse) {
          throw new ApiError(
            httpStatusFromGrpcCode(error.code),
            errorResponse.errors,
          );
        }
      }

      throw new ApiError(503, [
        {
          code: 'SERVICE_UNAVAILABLE',
          message: 'A required service is unavailable.',
        },
      ]);
    }
  }
}
