import { Logger } from '@nestjs/common';
import { createIdentityMicroservice } from './app/app.bootstrap';

async function bootstrap() {
  const grpcPort = Number(process.env.GRPC_PORT ?? 50051);
  const app = await createIdentityMicroservice(grpcPort);
  app.enableShutdownHooks();
  await app.listen();
  Logger.log(`Identity gRPC service is running on: 0.0.0.0:${grpcPort}`);
}

bootstrap();
