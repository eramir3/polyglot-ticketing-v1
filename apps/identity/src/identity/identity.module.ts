import { Module } from '@nestjs/common';
import { IdentityAuthModule } from '../auth/auth.module';
import { IdentityController } from './identity.controller';
import { IdentityService } from './identity.service';

@Module({
  imports: [IdentityAuthModule],
  controllers: [IdentityController],
  providers: [IdentityService],
})
export class IdentityModule {}
