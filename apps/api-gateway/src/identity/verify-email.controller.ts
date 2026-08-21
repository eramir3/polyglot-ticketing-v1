import { Controller, Get, Query } from '@nestjs/common';
import { VerifyEmailResponse } from './identity.types';
import { VerifyEmailDto } from './verify-email.dto';
import { VerifyEmailService } from './verify-email.service';

@Controller('auth')
export class VerifyEmailController {
  constructor(private readonly verifyEmailService: VerifyEmailService) {}

  @Get('verify-email')
  verifyEmail(@Query() dto: VerifyEmailDto): Promise<VerifyEmailResponse> {
    return this.verifyEmailService.verifyEmail(dto);
  }
}
