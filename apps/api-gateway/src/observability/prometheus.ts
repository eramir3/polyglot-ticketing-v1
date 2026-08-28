import { createServer, type Server } from 'node:http';
import {
  collectDefaultMetrics,
  Counter,
  Histogram,
  Registry,
} from '@prometheus-io/client';

export const metricsPort = Number(process.env.METRICS_PORT ?? 9090);

export class ApiGatewayMetrics {
  private readonly registry = new Registry();
  private readonly requests = new Counter({
    name: 'ticketing_app_http_server_requests_total',
    help: 'Total completed HTTP requests handled by the API gateway.',
    labelNames: ['method', 'route', 'status'] as const,
    registers: [this.registry],
  });
  private readonly duration = new Histogram({
    name: 'ticketing_app_http_server_request_duration_seconds',
    help: 'Duration of completed HTTP requests handled by the API gateway.',
    labelNames: ['method', 'route', 'status'] as const,
    registers: [this.registry],
  });

  constructor() {
    collectDefaultMetrics({ register: this.registry });
  }

  observeRequest(
    method: string,
    route: string,
    status: number,
    durationSeconds: number,
  ): void {
    const labels = { method, route, status: String(status) };
    this.requests.inc(labels);
    this.duration.observe(labels, durationSeconds);
  }

  contentType(): string {
    return this.registry.contentType;
  }

  metrics(): Promise<string> {
    return this.registry.metrics();
  }
}

export function startMetricsServer(metrics: ApiGatewayMetrics): Server {
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
