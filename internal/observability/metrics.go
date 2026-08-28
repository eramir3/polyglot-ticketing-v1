package observability

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

const DefaultMetricsPort = "9090"

// Metrics contains the bounded, service-local Prometheus metrics shared by the
// Go applications. The scrape target supplies the service label, so no metric
// label can contain user or aggregate identifiers.
type Metrics struct {
	backgroundDuration   *prometheus.HistogramVec
	backgroundOperations *prometheus.CounterVec
	grpcDuration         *prometheus.HistogramVec
	grpcRequests         *prometheus.CounterVec
}

func NewMetrics() (*Metrics, *prometheus.Registry) {
	registry := prometheus.NewRegistry()
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(prometheus.NewGoCollector())

	metrics := &Metrics{
		grpcRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ticketing_app_grpc_server_requests_total",
			Help: "Total completed gRPC server requests.",
		}, []string{"method", "code"}),
		grpcDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "ticketing_app_grpc_server_request_duration_seconds",
			Help: "Duration of completed gRPC server requests.",
		}, []string{"method", "code"}),
		backgroundOperations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ticketing_app_background_operations_total",
			Help: "Total completed background operations.",
		}, []string{"component", "operation", "outcome"}),
		backgroundDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "ticketing_app_background_operation_duration_seconds",
			Help: "Duration of completed background operations.",
		}, []string{"component", "operation", "outcome"}),
	}
	registry.MustRegister(
		metrics.grpcRequests,
		metrics.grpcDuration,
		metrics.backgroundOperations,
		metrics.backgroundDuration,
	)

	return metrics, registry
}

func (metrics *Metrics) UnaryServerInterceptor(
	ctx context.Context,
	request any,
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (any, error) {
	started := time.Now()
	response, err := handler(ctx, request)
	code := status.Code(err).String()
	metrics.grpcRequests.WithLabelValues(info.FullMethod, code).Inc()
	metrics.grpcDuration.WithLabelValues(info.FullMethod, code).Observe(time.Since(started).Seconds())
	return response, err
}

func (metrics *Metrics) ObserveBackground(component string, operation string, outcome string, started time.Time) {
	duration := time.Since(started).Seconds()
	metrics.backgroundOperations.WithLabelValues(component, operation, outcome).Inc()
	metrics.backgroundDuration.WithLabelValues(component, operation, outcome).Observe(duration)
}

func StartMetricsServer(ctx context.Context, port string, registry *prometheus.Registry, logger *slog.Logger) {
	server := &http.Server{
		Addr:              ":" + port,
		Handler:           MetricsHandler(registry),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("Prometheus metrics server stopped unexpectedly", "error", err)
		}
	}()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Warn("failed to stop Prometheus metrics server", "error", err)
		}
	}()
}

func MetricsHandler(registry *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	return mux
}
