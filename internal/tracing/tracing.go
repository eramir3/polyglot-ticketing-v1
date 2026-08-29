// Package tracing installs the local OpenTelemetry tracing pipeline and
// provides W3C Trace Context propagation for NATS headers.
package tracing

import (
	"context"
	"strings"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

func Install(ctx context.Context, serviceName string, endpoint string) (func(context.Context) error, error) {
	if endpoint == "" {
		endpoint = "localhost:4318"
	}
	exporter, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(resource.NewWithAttributes("", attribute.String("service.name", serviceName), attribute.String("deployment.environment", "local"))),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	return provider.Shutdown, nil
}

func InjectNATS(ctx context.Context, headers nats.Header) {
	otel.GetTextMapPropagator().Inject(ctx, natsCarrier{headers})
}

func ExtractNATS(ctx context.Context, headers nats.Header) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, natsCarrier{headers})
}

func ContextFromHeaders(ctx context.Context, traceparent, tracestate *string) context.Context {
	headers := nats.Header{}
	if traceparent != nil {
		headers.Set("traceparent", *traceparent)
	}
	if tracestate != nil {
		headers.Set("tracestate", *tracestate)
	}
	return ExtractNATS(ctx, headers)
}

func HeaderValues(ctx context.Context) (*string, *string) {
	headers := nats.Header{}
	InjectNATS(ctx, headers)
	traceparent := headers.Get("traceparent")
	if traceparent == "" {
		return nil, nil
	}
	tracestate := headers.Get("tracestate")
	if tracestate == "" {
		return &traceparent, nil
	}
	return &traceparent, &tracestate
}

type natsCarrier struct{ headers nats.Header }

func (carrier natsCarrier) Get(key string) string { return carrier.headers.Get(key) }
func (carrier natsCarrier) Set(key, value string) { carrier.headers.Set(key, value) }
func (carrier natsCarrier) Keys() []string {
	keys := make([]string, 0, len(carrier.headers))
	for key := range carrier.headers {
		keys = append(keys, key)
	}
	return keys
}
