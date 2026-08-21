import { join } from 'node:path';
import { Module } from '@nestjs/common';
import { ClientsModule, Transport } from '@nestjs/microservices';
import { IDENTITY_GRPC_CLIENT } from './identity.constants';
import { SignupController } from './signup.controller';
import { SignupService } from './signup.service';
import { VerifyEmailController } from './verify-email.controller';
import { VerifyEmailService } from './verify-email.service';

@Module({
  imports: [
    ClientsModule.register([
      {
        name: IDENTITY_GRPC_CLIENT,
        transport: Transport.GRPC,
        options: {
          loader: {
            includeDirs: [
              join(process.cwd(), 'proto'),
              join(process.cwd(), 'proto-deps'),
            ],
          },
          package: 'identity.v1',
          protoPath: join(process.cwd(), 'proto/identity/v1/identity.proto'),
          url: process.env.IDENTITY_GRPC_URL ?? 'localhost:50051',
        },
      },
    ]),
  ],
  controllers: [SignupController, VerifyEmailController],
  providers: [SignupService, VerifyEmailService],
})
export class IdentityModule {}
