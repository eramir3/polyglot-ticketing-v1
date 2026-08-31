import { headers } from '@nats-io/transport-node';
import { jest } from '@jest/globals';
import {
  expirationDeadLetterStream,
  orderCreatedDeadLetterSubject,
  orderCreatedSubject,
} from './expiration.constants.js';
import { JetStreamService } from './jetstream.service.js';
import type { OrderCreatedDelivery } from './expiration.types.js';

describe('JetStreamService expiration dead letter queue', () => {
  it('adds the dead-letter subject to an existing stream', async () => {
    const update = jest.fn();
    const service = serviceWithManager({
      streams: {
        info: async () => ({
          config: {
            name: expirationDeadLetterStream,
            subjects: ['dlq.expiration.legacy.v1'],
          },
        }),
        update,
      },
    });

    await asDeadLetterService(service).ensureExpirationDeadLetterStream();

    expect(update).toHaveBeenCalledWith(expirationDeadLetterStream, {
      name: expirationDeadLetterStream,
      subjects: [
        'dlq.expiration.legacy.v1',
        orderCreatedDeadLetterSubject,
      ],
    });
  });

  it('parks the original payload and delivery metadata', async () => {
    const publish = jest.fn().mockResolvedValue({ seq: 1 });
    const service = serviceWithClient({ publish });
    const originalHeaders = headers();
    originalHeaders.set('traceparent', '00-trace-parent');
    const delivery: OrderCreatedDelivery = {
      ack: () => undefined,
      data: new Uint8Array([1, 2, 3]),
      headers: originalHeaders,
      info: {
        deliveryCount: 6,
        stream: 'ORDERS_EVENTS',
        streamSequence: 42,
      },
      nak: () => undefined,
      subject: orderCreatedSubject,
    };

    await service.parkOrderCreated(
      delivery,
      'retryable',
      new Error('Redis\nunavailable'),
    );

    expect(publish).toHaveBeenCalledWith(
      orderCreatedDeadLetterSubject,
      delivery.data,
      expect.objectContaining({
        msgID: 'expiration-dlq-v1:ORDERS_EVENTS:42',
      }),
    );
    const options = publish.mock.calls[0]?.[2];
    expect(options.headers.get('traceparent')).toBe('00-trace-parent');
    expect(
      options.headers.get('X-Expiration-DLQ-Original-Subject'),
    ).toBe(orderCreatedSubject);
    expect(
      options.headers.get('X-Expiration-DLQ-Delivery-Count'),
    ).toBe('6');
    expect(
      options.headers.get('X-Expiration-DLQ-Failure-Reason'),
    ).toBe('Redis unavailable');
  });

  it('replays a retained OrderCreated event with trace headers', async () => {
    const publish = jest.fn().mockResolvedValue({ seq: 7 });
    const storedHeaders = headers();
    storedHeaders.set('traceparent', '00-trace-parent');
    storedHeaders.set('X-Expiration-DLQ-Original-Subject', orderCreatedSubject);
    storedHeaders.set('Nats-Msg-Id', 'expiration-dlq-v1:ORDERS_EVENTS:42');
    const service = serviceWithManagerAndClient(
      {
        streams: {
          getMessage: async () => ({
            data: new Uint8Array([1, 2, 3]),
            header: storedHeaders,
            subject: orderCreatedDeadLetterSubject,
          }),
        },
      },
      { publish },
    );

    await expect(service.replayExpirationDeadLetter(1)).resolves.toBe(7);

    expect(publish).toHaveBeenCalledWith(
      orderCreatedSubject,
      new Uint8Array([1, 2, 3]),
      expect.objectContaining({ msgID: 'expiration-dlq-replay-v1:1' }),
    );
    const options = publish.mock.calls[0]?.[2];
    expect(options.headers.get('traceparent')).toBe('00-trace-parent');
    expect(
      options.headers.get('X-Expiration-DLQ-Original-Subject'),
    ).toBe('');
  });

  it('rejects a replay with an unexpected original subject', async () => {
    const storedHeaders = headers();
    storedHeaders.set('X-Expiration-DLQ-Original-Subject', 'orders.unsupported.v1');
    const service = serviceWithManagerAndClient(
      {
        streams: {
          getMessage: async () => ({
            data: new Uint8Array(),
            header: storedHeaders,
            subject: orderCreatedDeadLetterSubject,
          }),
        },
      },
      { publish: jest.fn() },
    );

    await expect(service.replayExpirationDeadLetter(1)).rejects.toThrow(
      'unsupported original subject',
    );
  });
});

function serviceWithManager(manager: unknown): JetStreamService {
  const service = new JetStreamService();
  (service as unknown as { manager: unknown }).manager = manager;
  return service;
}

function serviceWithClient(client: unknown): JetStreamService {
  const service = new JetStreamService();
  (service as unknown as { client: unknown }).client = client;
  return service;
}

function serviceWithManagerAndClient(
  manager: unknown,
  client: unknown,
): JetStreamService {
  const service = serviceWithManager(manager);
  (service as unknown as { client: unknown }).client = client;
  return service;
}

function asDeadLetterService(service: JetStreamService): {
  ensureExpirationDeadLetterStream(): Promise<void>;
} {
  return service as unknown as {
    ensureExpirationDeadLetterStream(): Promise<void>;
  };
}
