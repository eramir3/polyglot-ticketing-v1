import { status } from '@grpc/grpc-js';
import {
  BadGatewayException,
  BadRequestException,
  ConflictException,
  Inject,
  Injectable,
  OnModuleInit,
} from '@nestjs/common';
import type { ClientGrpc } from '@nestjs/microservices';
import { firstValueFrom } from 'rxjs';
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
    @Inject(IDENTITY_GRPC_CLIENT) private readonly identityClient: ClientGrpc
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
        if (error.code === status.ALREADY_EXISTS) {
          throw new ConflictException(error.details ?? 'Email already exists.');
        }

        if (error.code === status.INVALID_ARGUMENT) {
          throw new BadRequestException(error.details ?? 'Invalid signup data.');
        }
      }

      throw new BadGatewayException('Identity service signup request failed.');
    }
  }
}

function isGrpcStatusError(
  error: unknown
): error is { code: number; details?: string } {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    typeof error.code === 'number'
  );
}
