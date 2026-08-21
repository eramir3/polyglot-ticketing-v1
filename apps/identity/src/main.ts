import { join } from 'node:path';
import { Logger } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { MicroserviceOptions, Transport } from '@nestjs/microservices';
import { AppModule } from './app/app.module';

async function bootstrap() {
  const grpcPort = Number(process.env.GRPC_PORT ?? 50051);
  const app = await NestFactory.createMicroservice<MicroserviceOptions>(
    AppModule,
    {
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
        url: `0.0.0.0:${grpcPort}`,
      },
    },
  );
  app.enableShutdownHooks();
  await app.listen();
  Logger.log(`Identity gRPC service is running on: 0.0.0.0:${grpcPort}`);
}

bootstrap();
