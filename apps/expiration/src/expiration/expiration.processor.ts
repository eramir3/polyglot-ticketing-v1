import { Processor, WorkerHost } from '@nestjs/bullmq';
import { Inject, Logger, Optional } from '@nestjs/common';
import { Job } from 'bullmq';
import {
  EXPIRATION_EVENT_PUBLISHER,
  expirationJobName,
  expirationQueueName,
} from './expiration.constants.js';
import {
  ExpirationEventPublisher,
  ExpirationJobData,
} from './expiration.types.js';
import { ExpirationMetrics } from '../observability/prometheus.js';

@Processor(expirationQueueName)
export class ExpirationProcessor extends WorkerHost {
  private readonly logger = new Logger(ExpirationProcessor.name);

  constructor(
    @Inject(EXPIRATION_EVENT_PUBLISHER)
    private readonly publisher: ExpirationEventPublisher,
    @Optional() private readonly metrics?: ExpirationMetrics,
  ) {
    super();
  }

  async process(job: Job<ExpirationJobData>): Promise<void> {
    const started = performance.now();
    if (job.name !== expirationJobName) {
      this.observe('unsupported', started);
      throw new Error(`Unsupported expiration job: ${job.name}`);
    }

    try {
      await this.publisher.publishExpirationComplete(job.data.orderId);
      this.observe('success', started);
    } catch (error) {
      this.logger.error(
        `expiration-complete publish failed; job will be retried order_id=${job.data.orderId}`,
        error,
      );
      this.observe('retry', started);
      throw error;
    }
  }

  private observe(outcome: string, started: number): void {
    this.metrics?.observe(
      'bullmq_processor',
      'expiration_complete',
      outcome,
      (performance.now() - started) / 1_000,
    );
  }
}
