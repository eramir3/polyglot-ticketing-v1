import { Body, Controller, Post } from '@nestjs/common';
import { SignUpRequest, SignUpResponse } from './identity.types';
import { SignupService } from './signup.service';

@Controller('identity')
export class SignupController {
  constructor(private readonly signupService: SignupService) {}

  @Post('signup')
  signUp(@Body() request: SignUpRequest): Promise<SignUpResponse> {
    return this.signupService.signUp(request);
  }
}
