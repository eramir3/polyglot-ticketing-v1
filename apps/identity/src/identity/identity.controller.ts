import { status } from '@grpc/grpc-js';
import { Controller } from '@nestjs/common';
import { GrpcMethod, RpcException } from '@nestjs/microservices';
import { SignUpRequest, SignUpResponse } from './identity.types';

@Controller()
export class IdentityController {
  @GrpcMethod('IdentityService', 'SignUp')
  signUp(_request: SignUpRequest): SignUpResponse {
    throw new RpcException({
      code: status.UNIMPLEMENTED,
      details: 'Identity signup is not configured yet.',
    });
  }
}
