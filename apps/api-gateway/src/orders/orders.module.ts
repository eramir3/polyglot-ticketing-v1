import { join } from 'node:path';
import { Module } from '@nestjs/common';
import { ClientsModule, Transport } from '@nestjs/microservices';
import { IdentityModule } from '../identity/identity.module';
import {
  ORDERS_GRPC_CLIENT,
  ORDERS_GRPC_LOADER_OPTIONS,
} from './orders.constants';
import { OrdersController } from './orders.controller';
import { OrdersService } from './orders.service';

@Module({
  imports: [
    IdentityModule,
    ClientsModule.register([
      {
        name: ORDERS_GRPC_CLIENT,
        transport: Transport.GRPC,
        options: {
          loader: {
            includeDirs: [join(process.cwd(), 'proto')],
            ...ORDERS_GRPC_LOADER_OPTIONS,
          },
          package: 'orders.v1',
          protoPath: join(process.cwd(), 'proto/orders/v1/orders.proto'),
          url: process.env.ORDERS_GRPC_URL ?? 'localhost:50053',
        },
      },
    ]),
  ],
  controllers: [OrdersController],
  providers: [OrdersService],
})
export class OrdersModule {}
