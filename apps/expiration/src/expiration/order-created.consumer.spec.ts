import { create, toBinary } from '@bufbuild/protobuf';
import { timestampFromDate } from '@bufbuild/protobuf/wkt';
import { jest } from '@jest/globals';
import { Logger } from '@nestjs/common';
import { OrderCreatedSchema } from '../../../../protogen/ts/orders/v1/events_pb.js';
import { OrderStatus } from '../../../../protogen/ts/orders/v1/orders_pb.js';
import { OrderCreatedConsumer } from './order-created.consumer.js';
import { OrderCreatedHandler } from './order-created.handler.js';
import { OrderCreatedDelivery } from './expiration.types.js';

describe('OrderCreatedConsumer', () => {
  it('schedules and acknowledges a valid OrderCreated event', async () => {
    const scheduler = new FakeScheduler();
    const consumer = new OrderCreatedConsumer(
      new OrderCreatedHandler(scheduler as never),
      {} as never,
    );
    const delivery = new FakeDelivery(validOrderCreatedPayload());

    await consumer.handleDelivery(delivery);

    expect(delivery.acknowledged).toBe(1);
    expect(delivery.negativelyAcknowledged).toBe(0);
    expect(delivery.terminated).toBe(0);
    expect(scheduler.calls).toEqual([
      {
        expiresAt: new Date('2026-08-27T12:15:00.000Z'),
        orderId: 'order-1',
      },
    ]);
  });

  it('terminates malformed OrderCreated events', async () => {
    const error = jest.spyOn(Logger.prototype, 'error').mockImplementation();
    const consumer = new OrderCreatedConsumer(
      new OrderCreatedHandler(new FakeScheduler() as never),
      {} as never,
    );
    const delivery = new FakeDelivery(new Uint8Array());

    await consumer.handleDelivery(delivery);

    expect(delivery.acknowledged).toBe(0);
    expect(delivery.negativelyAcknowledged).toBe(0);
    expect(delivery.terminated).toBe(1);
    expect(error).toHaveBeenCalledWith(
      'terminal OrderCreated event',
      'OrderCreated event is invalid.',
    );
    error.mockRestore();
  });

  it('negatively acknowledges transient scheduling failures', async () => {
    const schedulingError = new Error('Redis unavailable');
    const warn = jest.spyOn(Logger.prototype, 'warn').mockImplementation();
    const consumer = new OrderCreatedConsumer(
      {
        handle: async () => {
          throw schedulingError;
        },
      } as unknown as OrderCreatedHandler,
      {} as never,
    );
    const delivery = new FakeDelivery(validOrderCreatedPayload());

    await consumer.handleDelivery(delivery);

    expect(delivery.acknowledged).toBe(0);
    expect(delivery.negativelyAcknowledged).toBe(1);
    expect(delivery.terminated).toBe(0);
    expect(warn).toHaveBeenCalledWith(
      'order-created handling failed; event will be retried',
      schedulingError,
    );
    warn.mockRestore();
  });
});

function validOrderCreatedPayload(): Uint8Array {
  return toBinary(
    OrderCreatedSchema,
    create(OrderCreatedSchema, {
      eventId: 'event-1',
      expiresAt: timestampFromDate(new Date('2026-08-27T12:15:00.000Z')),
      orderId: 'order-1',
      orderStatus: OrderStatus.CREATED,
    }),
  );
}

class FakeScheduler {
  calls: Array<{ expiresAt: Date; orderId: string }> = [];

  async schedule(orderId: string, expiresAt: Date): Promise<void> {
    this.calls.push({ expiresAt, orderId });
  }
}

class FakeDelivery implements OrderCreatedDelivery {
  acknowledged = 0;
  negativelyAcknowledged = 0;
  terminated = 0;

  constructor(readonly data: Uint8Array) {}

  ack(): void {
    this.acknowledged += 1;
  }

  nak(): void {
    this.negativelyAcknowledged += 1;
  }

  term(): void {
    this.terminated += 1;
  }
}
