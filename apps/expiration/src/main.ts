import { Logger } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { AppModule } from './app/app.module.js';
import {
  ExpirationMetrics,
  startMetricsServer,
} from './observability/prometheus.js';

async function bootstrap(): Promise<void> {
  const app = await NestFactory.createApplicationContext(AppModule);
  app.enableShutdownHooks();
  const metricsServer = startMetricsServer(app.get(ExpirationMetrics));
  process.once('SIGINT', () => metricsServer.close());
  process.once('SIGTERM', () => metricsServer.close());
  Logger.log('Expiration service started');
}

void bootstrap();
