import { join } from 'node:path';
import { Logger } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { MicroserviceOptions, Transport } from '@nestjs/microservices';
import { AppModule } from './app/app.module';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  app.enableShutdownHooks();
  const globalPrefix = 'api';
  app.setGlobalPrefix(globalPrefix);
  const port = Number(process.env.PORT ?? 3001);
  const grpcPort = Number(process.env.GRPC_PORT ?? 50051);
  app.connectMicroservice<MicroserviceOptions>({
    transport: Transport.GRPC,
    options: {
      package: 'identity.v1',
      protoPath: join(process.cwd(), 'proto/identity/v1/identity.proto'),
      url: `0.0.0.0:${grpcPort}`,
    },
  });
  await app.startAllMicroservices();
  await app.listen(port);
  Logger.log(
    `Identity service is running on: http://localhost:${port}/${globalPrefix}`
  );
  Logger.log(`Identity gRPC service is running on: 0.0.0.0:${grpcPort}`);
}

bootstrap();
