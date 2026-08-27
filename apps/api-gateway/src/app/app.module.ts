import { Module } from '@nestjs/common';
import { IdentityModule } from '../identity/identity.module';
import { OrdersModule } from '../orders/orders.module';
import { PaymentsModule } from '../payments/payments.module';
import { TicketsModule } from '../tickets/tickets.module';

@Module({
  imports: [IdentityModule, TicketsModule, OrdersModule, PaymentsModule],
})
export class AppModule {}
