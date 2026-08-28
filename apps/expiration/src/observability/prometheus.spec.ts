import { describe, expect, it } from '@jest/globals';
import { ExpirationMetrics } from './prometheus.js';

describe('ExpirationMetrics', () => {
  it('exposes background operation metrics in Prometheus text format', async () => {
    const metrics = new ExpirationMetrics();
    metrics.observe('bullmq_processor', 'expiration_complete', 'retry', 0.05);

    await expect(metrics.metrics()).resolves.toContain(
      'ticketing_app_background_operations_total',
    );
  });
});
