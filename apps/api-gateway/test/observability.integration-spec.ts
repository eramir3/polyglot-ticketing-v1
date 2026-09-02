import {
  ApiGatewayMetrics,
  trafficSourceFor,
} from '../src/observability/prometheus';

describe('API gateway observability', () => {
  const token = 'load-test-metrics-token';

  it('classifies only a matching load-test token as k6 traffic', () => {
    expect(trafficSourceFor(token, token)).toBe('k6');
    expect(trafficSourceFor('incorrect-token', token)).toBe('other');
    expect(trafficSourceFor('aa', 'éé')).toBe('other');
    expect(trafficSourceFor(undefined, token)).toBe('other');
    expect(trafficSourceFor(token, '')).toBe('other');
  });

  it('exports the bounded traffic source label on gateway request metrics', async () => {
    const metrics = new ApiGatewayMetrics();
    metrics.observeRequest('GET', '/api/tickets', 200, 0.01, 'k6');
    metrics.observeRequest('GET', '/api/tickets', 200, 0.01, 'other');

    const output = await metrics.metrics();

    expect(output).toContain(
      'ticketing_app_http_server_requests_total{method="GET",route="/api/tickets",status="200",traffic_source="k6"} 1',
    );
    expect(output).toContain(
      'ticketing_app_http_server_requests_total{method="GET",route="/api/tickets",status="200",traffic_source="other"} 1',
    );
  });
});
