import { InjectQueue } from '@nestjs/bullmq';
import { Injectable } from '@nestjs/common';
import { Queue } from 'bullmq';
import {
  expirationJobName,
  expirationPublishAttempts,
  expirationPublishBackoffMilliseconds,
  expirationQueueName,
} from './expiration.constants.js';
import { ExpirationJobData } from './expiration.types.js';

@Injectable()
export class ExpirationScheduler {
  constructor(
    @InjectQueue(expirationQueueName)
    private readonly queue: Queue<ExpirationJobData>,
  ) {}

  async schedule(orderId: string, expiresAt: Date): Promise<void> {
    const delay = Math.max(0, expiresAt.getTime() - Date.now());

    await this.queue.add(
      expirationJobName,
      { orderId },
      {
        attempts: expirationPublishAttempts,
        backoff: {
          type: 'exponential',
          delay: expirationPublishBackoffMilliseconds,
        },
        delay,
        jobId: orderId,
        removeOnComplete: false,
        removeOnFail: false,
      },
    );
  }
}
