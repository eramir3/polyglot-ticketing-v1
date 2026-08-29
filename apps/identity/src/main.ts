import { Logger } from '@nestjs/common';
import { startTracing } from '../../../libs/observability/src/index.js';
import {
  GrpcMetricsInterceptor,
  IdentityMetrics,
  startMetricsServer,
} from './observability/prometheus';

async function bootstrap() {
  const shutdownTracing = startTracing('identity');
  const { createIdentityMicroservice } = await import('./app/app.bootstrap.js');
  const metrics = new IdentityMetrics();
  const metricsServer = startMetricsServer(metrics);
  const grpcPort = Number(process.env.GRPC_PORT ?? 50051);
  const app = await createIdentityMicroservice(grpcPort);
  app.enableShutdownHooks();
  app.useGlobalInterceptors(new GrpcMetricsInterceptor(metrics));
  await app.listen();
  process.once('SIGINT', () => metricsServer.close());
  process.once('SIGTERM', () => metricsServer.close());
  process.once('SIGINT', () => void shutdownTracing());
  process.once('SIGTERM', () => void shutdownTracing());
  Logger.log(`Identity gRPC service is running on: 0.0.0.0:${grpcPort}`);
}

bootstrap();
