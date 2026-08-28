import { Processor, WorkerHost } from '@nestjs/bullmq';
import { Inject, Logger } from '@nestjs/common';
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

@Processor(expirationQueueName)
export class ExpirationProcessor extends WorkerHost {
  private readonly logger = new Logger(ExpirationProcessor.name);

  constructor(
    @Inject(EXPIRATION_EVENT_PUBLISHER)
    private readonly publisher: ExpirationEventPublisher,
  ) {
    super();
  }

  async process(job: Job<ExpirationJobData>): Promise<void> {
    if (job.name !== expirationJobName) {
      throw new Error(`Unsupported expiration job: ${job.name}`);
    }

    try {
      await this.publisher.publishExpirationComplete(job.data.orderId);
    } catch (error) {
      this.logger.error(
        `expiration-complete publish failed; job will be retried order_id=${job.data.orderId}`,
        error,
      );
      throw error;
    }
  }
}
