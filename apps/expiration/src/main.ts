import { Logger } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { startTracing } from '../../../libs/observability/src/index.js';
import {
  ExpirationMetrics,
  startMetricsServer,
} from './observability/prometheus.js';

async function bootstrap(): Promise<void> {
  const shutdownTracing = startTracing('expiration');
  const { AppModule } = await import('./app/app.module.js');
  const app = await NestFactory.createApplicationContext(AppModule);
  app.enableShutdownHooks();
  const metricsServer = startMetricsServer(app.get(ExpirationMetrics));
  process.once('SIGINT', () => metricsServer.close());
  process.once('SIGTERM', () => metricsServer.close());
  process.once('SIGINT', () => void shutdownTracing());
  process.once('SIGTERM', () => void shutdownTracing());
  Logger.log('Expiration service started');
}

void bootstrap();
