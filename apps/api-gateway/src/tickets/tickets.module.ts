import { join } from 'node:path';
import { Module } from '@nestjs/common';
import { ClientsModule, Transport } from '@nestjs/microservices';
import { IdentityModule } from '../identity/identity.module';
import {
  TICKETS_GRPC_CLIENT,
  TICKETS_GRPC_LOADER_OPTIONS,
} from './tickets.constants';
import { TicketsController } from './tickets.controller';
import { TicketsService } from './tickets.service';

@Module({
  imports: [
    IdentityModule,
    ClientsModule.register([
      {
        name: TICKETS_GRPC_CLIENT,
        transport: Transport.GRPC,
        options: {
          loader: {
            includeDirs: [join(process.cwd(), 'proto')],
            ...TICKETS_GRPC_LOADER_OPTIONS,
          },
          package: 'tickets.v1',
          protoPath: join(process.cwd(), 'proto/tickets/v1/tickets.proto'),
          url: process.env.TICKETS_GRPC_URL ?? 'localhost:50052',
        },
      },
    ]),
  ],
  controllers: [TicketsController],
  providers: [TicketsService],
})
export class TicketsModule {}
