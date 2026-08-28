package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestUnaryServerInterceptorRecordsCompletedRequest(t *testing.T) {
	metrics, _ := NewMetrics()
	_, err := metrics.UnaryServerInterceptor(
		context.Background(),
		nil,
		&grpc.UnaryServerInfo{FullMethod: "/tickets.v1.TicketsService/CreateTicket"},
		func(context.Context, any) (any, error) {
			return nil, status.Error(codes.Internal, "database unavailable")
		},
	)
	if status.Code(err) != codes.Internal {
		t.Fatalf("expected internal error, got %v", err)
	}

	if got := testutil.ToFloat64(metrics.grpcRequests.WithLabelValues("/tickets.v1.TicketsService/CreateTicket", "Internal")); got != 1 {
		t.Fatalf("expected one completed request, got %v", got)
	}
	if got := testutil.CollectAndCount(metrics.grpcDuration); got != 1 {
		t.Fatalf("expected one duration series, got %d", got)
	}
}

func TestObserveBackgroundRecordsOutcomeAndDuration(t *testing.T) {
	metrics, _ := NewMetrics()
	metrics.ObserveBackground("jetstream_consumer", "ticket_projection", "retry", time.Now().Add(-time.Millisecond))

	if got := testutil.ToFloat64(metrics.backgroundOperations.WithLabelValues("jetstream_consumer", "ticket_projection", "retry")); got != 1 {
		t.Fatalf("expected one completed background operation, got %v", got)
	}
	if got := testutil.CollectAndCount(metrics.backgroundDuration); got != 1 {
		t.Fatalf("expected one background duration series, got %d", got)
	}
}

func TestMetricsHandlerExposesPrometheusText(t *testing.T) {
	metrics, registry := NewMetrics()
	metrics.ObserveBackground("jetstream_consumer", "ticket_projection", "success", time.Now())

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	response := httptest.NewRecorder()
	MetricsHandler(registry).ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if !strings.Contains(response.Body.String(), "ticketing_app_background_operations_total") {
		t.Fatalf("expected Prometheus text exposition, got %q", response.Body.String())
	}
}

func TestMetricsHandlerRejectsOtherPaths(t *testing.T) {
	_, registry := NewMetrics()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	MetricsHandler(registry).ServeHTTP(response, request)

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}
}
