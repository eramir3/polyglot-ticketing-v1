import { createServer, type Server } from 'node:http';
import {
  CallHandler,
  ExecutionContext,
  Injectable,
  NestInterceptor,
} from '@nestjs/common';
import {
  collectDefaultMetrics,
  Counter,
  Histogram,
  Registry,
} from '@prometheus-io/client';
import { Observable, catchError, finalize, throwError } from 'rxjs';

export const metricsPort = Number(process.env.METRICS_PORT ?? 9090);

export class IdentityMetrics {
  private readonly registry = new Registry();
  private readonly requests = new Counter({
    name: 'ticketing_app_grpc_server_requests_total',
    help: 'Total completed gRPC requests handled by Identity.',
    labelNames: ['method', 'code'] as const,
    registers: [this.registry],
  });
  private readonly duration = new Histogram({
    name: 'ticketing_app_grpc_server_request_duration_seconds',
    help: 'Duration of completed gRPC requests handled by Identity.',
    labelNames: ['method', 'code'] as const,
    registers: [this.registry],
  });

  constructor() {
    collectDefaultMetrics({ register: this.registry });
  }

  observeRequest(method: string, code: string, durationSeconds: number): void {
    const labels = { method, code };
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

@Injectable()
export class GrpcMetricsInterceptor implements NestInterceptor {
  constructor(private readonly metrics: IdentityMetrics) {}

  intercept(context: ExecutionContext, next: CallHandler): Observable<unknown> {
    const started = performance.now();
    const method = `${context.getClass().name}.${context.getHandler().name}`;
    let code = 'OK';

    return next.handle().pipe(
      catchError((error: { code?: number }) => {
        code = grpcCode(error.code);
        return throwError(() => error);
      }),
      finalize(() => {
        this.metrics.observeRequest(
          method,
          code,
          (performance.now() - started) / 1_000,
        );
      }),
    );
  }
}

export function startMetricsServer(metrics: IdentityMetrics): Server {
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

function grpcCode(code: number | undefined): string {
  const codes = [
    'OK',
    'Canceled',
    'Unknown',
    'InvalidArgument',
    'DeadlineExceeded',
    'NotFound',
    'AlreadyExists',
    'PermissionDenied',
    'ResourceExhausted',
    'FailedPrecondition',
    'Aborted',
    'OutOfRange',
    'Unimplemented',
    'Internal',
    'Unavailable',
    'DataLoss',
    'Unauthenticated',
  ];
  return code === undefined || code < 0 || code >= codes.length
    ? 'Unknown'
    : codes[code];
}
