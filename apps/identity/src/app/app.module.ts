import { Module } from '@nestjs/common';
import { IdentityController } from '../identity/identity.controller';
import { AppController } from './app.controller';
import { AppService } from './app.service';

@Module({
  imports: [],
  controllers: [AppController, IdentityController],
  providers: [AppService],
})
export class AppModule {}
