import { status } from '@grpc/grpc-js';
import {
  BadGatewayException,
  Inject,
  Injectable,
  NotImplementedException,
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
      if (isGrpcStatusError(error) && error.code === status.UNIMPLEMENTED) {
        throw new NotImplementedException(
          'Identity signup is not configured yet'
        );
      }

      throw new BadGatewayException('Identity service signup request failed.');
    }
  }
}

function isGrpcStatusError(error: unknown): error is { code: number } {
  return (
    typeof error === 'object' &&
    error !== null &&
    'code' in error &&
    typeof error.code === 'number'
  );
}
