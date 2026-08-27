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
});

class FakePublisher implements ExpirationEventPublisher {
  orderIds: string[] = [];

  async publishExpirationComplete(orderId: string): Promise<void> {
    this.orderIds.push(orderId);
  }
}
