import { join } from 'node:path';
import { Module } from '@nestjs/common';
import { ClientsModule, Transport } from '@nestjs/microservices';
import { IdentityModule } from '../identity/identity.module';
import {
  PAYMENTS_GRPC_CLIENT,
  PAYMENTS_GRPC_LOADER_OPTIONS,
} from './payments.constants';
import { PaymentsController } from './payments.controller';
import { PaymentsService } from './payments.service';

@Module({
  imports: [
    IdentityModule,
    ClientsModule.register([
      {
        name: PAYMENTS_GRPC_CLIENT,
        transport: Transport.GRPC,
        options: {
          loader: {
            includeDirs: [join(process.cwd(), 'proto')],
            ...PAYMENTS_GRPC_LOADER_OPTIONS,
          },
          package: 'payments.v1',
          protoPath: join(process.cwd(), 'proto/payments/v1/payments.proto'),
          url: process.env.PAYMENTS_GRPC_URL ?? 'localhost:50054',
        },
      },
    ]),
  ],
  controllers: [PaymentsController],
  providers: [PaymentsService],
})
export class PaymentsModule {}
