import { ExpirationScheduler } from './expiration.scheduler.js';
import { jest } from '@jest/globals';
import {
  expirationJobName,
  expirationPublishAttempts,
  expirationPublishBackoffMilliseconds,
} from './expiration.constants.js';
import { ExpirationQueue } from './expiration.types.js';
import type { JobsOptions } from 'bullmq';

describe('ExpirationScheduler', () => {
  afterEach(() => {
    jest.restoreAllMocks();
  });

  it('queues a retained, retryable job at the order deadline', async () => {
    const queue = new FakeExpirationQueue();
    const scheduler = new ExpirationScheduler(queue as never);
    jest.spyOn(Date, 'now').mockReturnValue(1_000);

    await scheduler.schedule('order-1', new Date(16_000));

    expect(queue.addCalls).toEqual([
      {
        data: { orderId: 'order-1' },
        name: expirationJobName,
        options: {
          attempts: expirationPublishAttempts,
          backoff: {
            type: 'exponential',
            delay: expirationPublishBackoffMilliseconds,
          },
          delay: 15_000,
          jobId: 'order-1',
          removeOnComplete: false,
          removeOnFail: false,
        },
      },
    ]);
  });

  it('queues an overdue order without an additional delay', async () => {
    const queue = new FakeExpirationQueue();
    const scheduler = new ExpirationScheduler(queue as never);
    jest.spyOn(Date, 'now').mockReturnValue(16_000);

    await scheduler.schedule('order-1', new Date(1_000));

    expect(queue.addCalls[0]?.options.delay).toBe(0);
  });
});

class FakeExpirationQueue implements ExpirationQueue {
  addCalls: Array<{
    data: { orderId: string };
    name: string;
    options: JobsOptions;
  }> = [];

  async add(
    name: string,
    data: { orderId: string },
    options: JobsOptions,
  ): Promise<void> {
    this.addCalls.push({ data, name, options });
  }
}
