import { OTLPTraceExporter } from '@opentelemetry/exporter-trace-otlp-http';
import { GrpcInstrumentation } from '@opentelemetry/instrumentation-grpc';
import { HttpInstrumentation } from '@opentelemetry/instrumentation-http';
import { resourceFromAttributes } from '@opentelemetry/resources';
import { NodeSDK } from '@opentelemetry/sdk-node';

export function startTracing(serviceName: string): () => Promise<void> {
  const endpoint = process.env.OTEL_EXPORTER_OTLP_ENDPOINT ?? 'localhost:4318';
  const sdk = new NodeSDK({
    resource: resourceFromAttributes({
      'service.name': serviceName,
      'deployment.environment': 'local',
    }),
    traceExporter: new OTLPTraceExporter({
      url: `http://${endpoint}/v1/traces`,
    }),
    instrumentations: [
      new HttpInstrumentation({
        ignoreIncomingRequestHook: (request) => request.url === '/metrics',
      }),
      new GrpcInstrumentation(),
    ],
  });
  sdk.start();
  return () => sdk.shutdown();
}
