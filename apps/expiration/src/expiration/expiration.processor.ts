import { Processor, WorkerHost } from '@nestjs/bullmq';
import { Inject } from '@nestjs/common';
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

    await this.publisher.publishExpirationComplete(job.data.orderId);
  }
}
