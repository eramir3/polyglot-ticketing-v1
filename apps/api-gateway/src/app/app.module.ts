import { Module } from '@nestjs/common';
import { IdentityModule } from '../identity/identity.module';
import { TicketsModule } from '../tickets/tickets.module';

@Module({
  imports: [IdentityModule, TicketsModule],
})
export class AppModule {}
