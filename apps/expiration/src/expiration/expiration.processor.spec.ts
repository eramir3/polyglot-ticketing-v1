import { describe, expect, it, jest } from '@jest/globals';
import { Logger } from '@nestjs/common';
import { ExpirationProcessor } from './expiration.processor.js';
import { expirationJobName } from './expiration.constants.js';
import { ExpirationEventPublisher } from './expiration.types.js';

describe('ExpirationProcessor', () => {
  it('publishes completion for the expired order', async () => {
    const publisher = new FakePublisher();
    const processor = new ExpirationProcessor(publisher);

    await processor.process({
      data: { orderId: 'order-1' },
      name: expirationJobName,
    } as never);

    expect(publisher.orderIds).toEqual(['order-1']);
  });

  it('throws unsupported jobs so BullMQ can fail them', async () => {
    const processor = new ExpirationProcessor(new FakePublisher());

    await expect(
      processor.process({
        data: { orderId: 'order-1' },
        name: 'unknown',
      } as never),
    ).rejects.toThrow('Unsupported expiration job');
  });

  it('logs and rethrows publish failures so BullMQ retries the job', async () => {
    const publishingError = new Error('NATS unavailable');
    const error = jest.spyOn(Logger.prototype, 'error').mockImplementation();
    const processor = new ExpirationProcessor({
      publishExpirationComplete: async () => {
        throw publishingError;
      },
    });

    await expect(
      processor.process({
        data: { orderId: 'order-1' },
        name: expirationJobName,
      } as never),
    ).rejects.toThrow(publishingError);

    expect(error).toHaveBeenCalledWith(
      'expiration-complete publish failed; job will be retried order_id=order-1',
      publishingError,
    );
    error.mockRestore();
  });
});

class FakePublisher implements ExpirationEventPublisher {
  orderIds: string[] = [];

  async publishExpirationComplete(orderId: string): Promise<void> {
    this.orderIds.push(orderId);
  }
}
