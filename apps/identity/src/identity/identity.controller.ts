import { Controller } from '@nestjs/common';
import { GrpcMethod } from '@nestjs/microservices';
import { IdentityService } from './identity.service';
import {
  SignUpRequest,
  SignUpResponse,
  VerifyEmailRequest,
  VerifyEmailResponse,
} from './identity.types';

@Controller()
export class IdentityController {
  constructor(private readonly identityService: IdentityService) {}

  @GrpcMethod('IdentityService', 'SignUp')
  signUp(request: SignUpRequest): Promise<SignUpResponse> {
    return this.identityService.signUp(request);
  }

  @GrpcMethod('IdentityService', 'VerifyEmail')
  verifyEmail(request: VerifyEmailRequest): Promise<VerifyEmailResponse> {
    return this.identityService.verifyEmail(request);
  }
}
