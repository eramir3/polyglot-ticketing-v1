import { createServer, type Server } from 'node:http';
import { Injectable } from '@nestjs/common';
import {
  collectDefaultMetrics,
  Counter,
  Histogram,
  Registry,
} from '@prometheus-io/client';

export const metricsPort = Number(process.env.METRICS_PORT ?? 9090);

@Injectable()
export class ExpirationMetrics {
  private readonly registry = new Registry();
  private readonly operations = new Counter({
    name: 'ticketing_app_background_operations_total',
    help: 'Total completed expiration background operations.',
    labelNames: ['component', 'operation', 'outcome'] as const,
    registers: [this.registry],
  });
  private readonly duration = new Histogram({
    name: 'ticketing_app_background_operation_duration_seconds',
    help: 'Duration of completed expiration background operations.',
    labelNames: ['component', 'operation', 'outcome'] as const,
    registers: [this.registry],
  });

  constructor() {
    collectDefaultMetrics({ register: this.registry });
  }

  observe(
    component: string,
    operation: string,
    outcome: string,
    durationSeconds: number,
  ): void {
    const labels = { component, operation, outcome };
    this.operations.inc(labels);
    this.duration.observe(labels, durationSeconds);
  }

  contentType(): string {
    return this.registry.contentType;
  }

  metrics(): Promise<string> {
    return this.registry.metrics();
  }
}

export function startMetricsServer(metrics: ExpirationMetrics): Server {
  const server = createServer(async (request, response) => {
    if (request.url !== '/metrics') {
      response.statusCode = 404;
      response.end();
      return;
    }

    response.setHeader('Content-Type', metrics.contentType());
    response.end(await metrics.metrics());
  });
  server.listen(metricsPort, '0.0.0.0');
  return server;
}
