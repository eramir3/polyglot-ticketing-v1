import { Body, Controller, Get, Post, Query } from '@nestjs/common';
import { SignUpResponse, VerifyEmailResponse } from './identity.types';
import { AuthService } from './auth.service';
import { SignUpDto } from './dtos/signup.dto';
import { VerifyEmailDto } from './dtos/verify-email.dto';

@Controller('auth')
export class AuthController {
  constructor(private readonly authService: AuthService) {}

  @Post('signup')
  signUp(@Body() dto: SignUpDto): Promise<SignUpResponse> {
    return this.authService.signUp(dto);
  }

  @Get('verify-email')
  verifyEmail(@Query() dto: VerifyEmailDto): Promise<VerifyEmailResponse> {
    return this.authService.verifyEmail(dto);
  }
}
