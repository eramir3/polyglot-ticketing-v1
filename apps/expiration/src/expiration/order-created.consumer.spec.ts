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
    expect(scheduler.calls).toEqual([
      {
        expiresAt: new Date('2026-08-27T12:15:00.000Z'),
        orderId: 'order-1',
      },
    ]);
  });

  it('parks malformed OrderCreated events', async () => {
    const jetStream = new FakeJetStreamService();
    const consumer = new OrderCreatedConsumer(
      new OrderCreatedHandler(new FakeScheduler() as never),
      jetStream as never,
    );
    const delivery = new FakeDelivery(new Uint8Array());

    await consumer.handleDelivery(delivery);

    expect(delivery.acknowledged).toBe(1);
    expect(delivery.negativelyAcknowledged).toBe(0);
    expect(jetStream.calls).toEqual([
      {
        delivery,
        failureClass: 'invalid',
        failure: expect.any(Error),
      },
    ]);
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
    expect(warn).toHaveBeenCalledWith(
      'order-created handling failed; event will be retried',
      schedulingError,
    );
    warn.mockRestore();
  });

  it('parks the sixth transient scheduling failure', async () => {
    const jetStream = new FakeJetStreamService();
    const consumer = new OrderCreatedConsumer(
      {
        handle: async () => {
          throw new Error('Redis unavailable');
        },
      } as unknown as OrderCreatedHandler,
      jetStream as never,
    );
    const delivery = new FakeDelivery(validOrderCreatedPayload(), 6);

    await consumer.handleDelivery(delivery);

    expect(delivery.acknowledged).toBe(1);
    expect(delivery.negativelyAcknowledged).toBe(0);
    expect(jetStream.calls[0]?.failureClass).toBe('retryable');
  });

  it('retries when dead-letter parking fails', async () => {
    const parkingError = new Error('NATS unavailable');
    const error = jest.spyOn(Logger.prototype, 'error').mockImplementation();
    const consumer = new OrderCreatedConsumer(
      new OrderCreatedHandler(new FakeScheduler() as never),
      new FakeJetStreamService(parkingError) as never,
    );
    const delivery = new FakeDelivery(new Uint8Array());

    await consumer.handleDelivery(delivery);

    expect(delivery.acknowledged).toBe(0);
    expect(delivery.negativelyAcknowledged).toBe(1);
    expect(error).toHaveBeenCalledWith(
      'failed to park OrderCreated event in dead letter queue; event will be retried',
      parkingError,
    );
    error.mockRestore();
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

class FakeJetStreamService {
  calls: Array<{
    delivery: OrderCreatedDelivery;
    failure: unknown;
    failureClass: string;
  }> = [];

  constructor(private readonly error?: Error) {}

  async parkOrderCreated(
    delivery: OrderCreatedDelivery,
    failureClass: string,
    failure: unknown,
  ): Promise<void> {
    if (this.error !== undefined) {
      throw this.error;
    }
    this.calls.push({ delivery, failure, failureClass });
  }
}

class FakeDelivery implements OrderCreatedDelivery {
  acknowledged = 0;
  negativelyAcknowledged = 0;
  readonly info;
  readonly subject = 'orders.order.created.v1';

  constructor(
    readonly data: Uint8Array,
    deliveryCount = 1,
  ) {
    this.info = {
      deliveryCount,
      stream: 'ORDERS_EVENTS',
      streamSequence: 1,
    };
  }

  ack(): void {
    this.acknowledged += 1;
  }

  nak(): void {
    this.negativelyAcknowledged += 1;
  }
}
