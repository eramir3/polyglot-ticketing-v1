import { Module } from '@nestjs/common';
import { IdentityModule } from '../identity/identity.module';
import { AppController } from './app.controller';
import { AppService } from './app.service';

@Module({
  imports: [IdentityModule],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}
