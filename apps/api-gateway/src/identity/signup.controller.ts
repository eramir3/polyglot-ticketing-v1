import { Body, Controller, Post } from '@nestjs/common';
import { SignUpResponse } from './identity.types';
import { SignUpDto } from './signup.dto';
import { SignupService } from './signup.service';

@Controller('auth')
export class SignupController {
  constructor(private readonly signupService: SignupService) {}

  @Post('signup')
  signUp(@Body() dto: SignUpDto): Promise<SignUpResponse> {
    return this.signupService.signUp(dto);
  }
}
