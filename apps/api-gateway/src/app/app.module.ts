import { Module } from '@nestjs/common';
import { IdentityModule } from '../identity/identity.module';
import { TicketsModule } from '../tickets/tickets.module';
import { AppController } from './app.controller';
import { AppService } from './app.service';

@Module({
  imports: [IdentityModule, TicketsModule],
  controllers: [AppController],
  providers: [AppService],
})
export class AppModule {}
