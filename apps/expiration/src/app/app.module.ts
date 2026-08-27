import { Module } from '@nestjs/common';
import { ExpirationModule } from '../expiration/expiration.module.js';

@Module({
  imports: [ExpirationModule],
})
export class AppModule {}
