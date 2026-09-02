import { timingSafeEqual } from 'node:crypto';
import { createServer, type Server } from 'node:http';
import {
  collectDefaultMetrics,
  Counter,
  Histogram,
  Registry,
} from '@prometheus-io/client';

export const metricsPort = Number(process.env.METRICS_PORT ?? 9090);

export type TrafficSource = 'k6' | 'other';

export function trafficSourceFor(
  suppliedToken: string | string[] | undefined,
  configuredToken: string,
): TrafficSource {
  if (configuredToken === '' || typeof suppliedToken !== 'string') {
    return 'other';
  }

  const supplied = Buffer.from(suppliedToken);
  const configured = Buffer.from(configuredToken);
  if (supplied.length !== configured.length) {
    return 'other';
  }

  return timingSafeEqual(supplied, configured) ? 'k6' : 'other';
}

export class ApiGatewayMetrics {
  private readonly registry = new Registry();
  private readonly requests = new Counter({
    name: 'ticketing_app_http_server_requests_total',
    help: 'Total completed HTTP requests handled by the API gateway.',
    labelNames: ['method', 'route', 'status', 'traffic_source'] as const,
    registers: [this.registry],
  });
  private readonly duration = new Histogram({
    name: 'ticketing_app_http_server_request_duration_seconds',
    help: 'Duration of completed HTTP requests handled by the API gateway.',
    labelNames: ['method', 'route', 'status', 'traffic_source'] as const,
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
    trafficSource: TrafficSource = 'other',
  ): void {
    const labels = {
      method,
      route,
      status: String(status),
      traffic_source: trafficSource,
    };
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
