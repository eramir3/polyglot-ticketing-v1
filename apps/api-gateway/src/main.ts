import { Logger } from '@nestjs/common';
import { startTracing } from '../../../libs/observability/src/index.js';
import {
  ApiGatewayMetrics,
  startMetricsServer,
  trafficSourceFor,
} from './observability/prometheus';

async function bootstrap() {
  const shutdownTracing = startTracing('api-gateway');
  const { createApiGatewayApplication } = await import('./app/app.bootstrap.js');
  const metrics = new ApiGatewayMetrics();
  const loadTestMetricsToken = process.env.LOAD_TEST_METRICS_TOKEN ?? '';
  const metricsServer = startMetricsServer(metrics);
  const app = await createApiGatewayApplication();
  app.enableShutdownHooks();
  app.use((request: any, response: any, next: () => void) => {
    const started = performance.now();
    response.once('finish', () => {
      const route =
        typeof request.route?.path === 'string'
          ? request.route.path
          : 'unmatched';
      metrics.observeRequest(
        request.method,
        route,
        response.statusCode,
        (performance.now() - started) / 1_000,
        trafficSourceFor(
          request.headers['x-ticketing-load-test-token'],
          loadTestMetricsToken,
        ),
      );
    });
    next();
  });
  const globalPrefix = 'api';
  const port = Number(process.env.PORT ?? 3000);
  await app.listen(port);
  process.once('SIGINT', () => metricsServer.close());
  process.once('SIGTERM', () => metricsServer.close());
  process.once('SIGINT', () => void shutdownTracing());
  process.once('SIGTERM', () => void shutdownTracing());
  Logger.log(
    `API gateway is running on: http://localhost:${port}/${globalPrefix}`,
  );
}

bootstrap();
