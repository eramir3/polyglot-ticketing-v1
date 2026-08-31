import {
  AckPolicy,
  DeliverPolicy,
  JetStreamApiError,
  JetStreamApiCodes,
  RetentionPolicy,
  StorageType,
  jetstream,
  jetstreamManager,
  type Consumer,
  type JetStreamClient,
  type JetStreamManager,
} from '@nats-io/jetstream';
import {
  connect,
  headers,
  nanos,
  type MsgHdrs,
  type NatsConnection,
} from '@nats-io/transport-node';
import {
  Injectable,
  Logger,
  OnApplicationShutdown,
  OnModuleInit,
} from '@nestjs/common';
import {
  expirationEventsStream,
  expirationDeadLetterStream,
  natsRetryMilliseconds,
  orderCreatedDeadLetterSubject,
  orderCreatedDurableName,
  orderCreatedSubject,
  ordersEventsStream,
} from './expiration.constants.js';
import type { OrderCreatedDelivery } from './expiration.types.js';

const deadLetterRetentionMilliseconds = 7 * 24 * 60 * 60 * 1_000;
const deadLetterHeader = 'X-Expiration-DLQ-';

@Injectable()
export class JetStreamService implements OnModuleInit, OnApplicationShutdown {
  private readonly logger = new Logger(JetStreamService.name);
  private connection?: NatsConnection;
  private client?: JetStreamClient;
  private manager?: JetStreamManager;

  async onModuleInit(): Promise<void> {
    for (;;) {
      try {
        this.connection = await connect({
          servers: process.env.NATS_URL ?? 'nats://localhost:4222',
          timeout: 5_000,
        });
        this.client = jetstream(this.connection);
        this.manager = await jetstreamManager(this.connection);
        await this.ensureExpirationEventStream();
        await this.ensureExpirationDeadLetterStream();
        return;
      } catch (error) {
        await this.connection?.close();
        this.connection = undefined;
        this.client = undefined;
        this.manager = undefined;
        this.logger.warn('JetStream is unavailable; reconnecting', error);
        await waitForNatsRetry();
      }
    }
  }

  async onApplicationShutdown(): Promise<void> {
    await this.connection?.drain();
  }

  async orderCreatedConsumer(): Promise<Consumer> {
    const manager = this.requireManager();

    try {
      await manager.consumers.info(ordersEventsStream, orderCreatedDurableName);
    } catch (error) {
      if (!isConsumerNotFound(error)) {
        throw error;
      }
      await manager.consumers.add(ordersEventsStream, {
        ack_policy: AckPolicy.Explicit,
        deliver_policy: DeliverPolicy.All,
        durable_name: orderCreatedDurableName,
        filter_subject: orderCreatedSubject,
      });
    }

    return this.requireClient().consumers.get(
      ordersEventsStream,
      orderCreatedDurableName,
    );
  }

  async publish(
    subject: string,
    payload: Uint8Array,
    messageId: string,
    messageHeaders?: MsgHdrs,
  ): Promise<void> {
    await this.requireClient().publish(subject, payload, {
      headers: messageHeaders,
      msgID: messageId,
    });
  }

  async parkOrderCreated(
    delivery: OrderCreatedDelivery,
    failureClass: string,
    failure: unknown,
  ): Promise<void> {
    const messageHeaders = cloneHeaders(delivery.headers);
    messageHeaders.set(
      `${deadLetterHeader}Original-Subject`,
      delivery.subject,
    );
    messageHeaders.set(
      `${deadLetterHeader}Original-Stream`,
      delivery.info.stream,
    );
    messageHeaders.set(
      `${deadLetterHeader}Original-Stream-Sequence`,
      String(delivery.info.streamSequence),
    );
    messageHeaders.set(
      `${deadLetterHeader}Consumer`,
      orderCreatedDurableName,
    );
    messageHeaders.set(
      `${deadLetterHeader}Delivery-Count`,
      String(delivery.info.deliveryCount),
    );
    messageHeaders.set(`${deadLetterHeader}Failure-Class`, failureClass);
    messageHeaders.set(
      `${deadLetterHeader}Failure-Reason`,
      sanitizeHeaderValue(failure),
    );
    messageHeaders.set(
      `${deadLetterHeader}Parked-At`,
      new Date().toISOString(),
    );

    await this.publish(
      orderCreatedDeadLetterSubject,
      delivery.data,
      `expiration-dlq-v1:${delivery.info.stream}:${delivery.info.streamSequence}`,
      messageHeaders,
    );
  }

  async replayExpirationDeadLetter(sequence: number): Promise<number> {
    const message = await this.requireManager().streams.getMessage(
      expirationDeadLetterStream,
      { seq: sequence },
    );
    if (message === null || message.subject !== orderCreatedDeadLetterSubject) {
      throw new Error(`message ${sequence} is not an Expiration DLQ message`);
    }

    const subject = message.header.get(`${deadLetterHeader}Original-Subject`);
    if (subject !== orderCreatedSubject) {
      throw new Error(
        `message ${sequence} has unsupported original subject ${JSON.stringify(subject)}`,
      );
    }

    const acknowledgement = await this.requireClient().publish(
      subject,
      message.data,
      {
        headers: withoutDeadLetterHeaders(message.header),
        msgID: `expiration-dlq-replay-v1:${sequence}`,
      },
    );
    return acknowledgement.seq;
  }

  private async ensureExpirationEventStream(): Promise<void> {
    const manager = this.requireManager();

    try {
      await manager.streams.info(expirationEventsStream);
      return;
    } catch (error) {
      if (!isStreamNotFound(error)) {
        throw error;
      }
    }

    try {
      await manager.streams.add({
        name: expirationEventsStream,
        retention: RetentionPolicy.Limits,
        storage: StorageType.File,
        subjects: ['expiration.>'],
      });
    } catch (error) {
      try {
        await manager.streams.info(expirationEventsStream);
      } catch {
        throw error;
      }
    }
  }

  private async ensureExpirationDeadLetterStream(): Promise<void> {
    const manager = this.requireManager();

    try {
      const information = await manager.streams.info(expirationDeadLetterStream);
      if (
        information.config.subjects.includes(orderCreatedDeadLetterSubject)
      ) {
        return;
      }
      await manager.streams.update(expirationDeadLetterStream, {
        ...information.config,
        subjects: [
          ...information.config.subjects,
          orderCreatedDeadLetterSubject,
        ],
      });
      return;
    } catch (error) {
      if (!isStreamNotFound(error)) {
        throw error;
      }
    }

    try {
      await manager.streams.add({
        duplicate_window: nanos(deadLetterRetentionMilliseconds),
        max_age: nanos(deadLetterRetentionMilliseconds),
        name: expirationDeadLetterStream,
        retention: RetentionPolicy.Limits,
        storage: StorageType.File,
        subjects: [orderCreatedDeadLetterSubject],
      });
    } catch (error) {
      try {
        await manager.streams.info(expirationDeadLetterStream);
      } catch {
        throw error;
      }
    }
  }

  private requireClient(): JetStreamClient {
    if (this.client === undefined) {
      throw new Error('JetStream client is not connected.');
    }
    return this.client;
  }

  private requireManager(): JetStreamManager {
    if (this.manager === undefined) {
      throw new Error('JetStream manager is not connected.');
    }
    return this.manager;
  }
}

function cloneHeaders(original?: MsgHdrs): MsgHdrs {
  const copy = headers();
  for (const [key, values] of original ?? []) {
    for (const value of values) {
      copy.append(key, value);
    }
  }
  return copy;
}

function withoutDeadLetterHeaders(original: MsgHdrs): MsgHdrs {
  const copy = headers();
  for (const [key, values] of original) {
    if (
      key.startsWith(deadLetterHeader) ||
      key.toLowerCase() === 'nats-msg-id'
    ) {
      continue;
    }
    for (const value of values) {
      copy.append(key, value);
    }
  }
  return copy;
}

function sanitizeHeaderValue(failure: unknown): string {
  const value = failure instanceof Error ? failure.message : String(failure);
  return value.replace(/[\r\n]/g, ' ');
}

function isStreamNotFound(error: unknown): boolean {
  return (
    error instanceof JetStreamApiError &&
    error.code === JetStreamApiCodes.StreamNotFound
  );
}

function isConsumerNotFound(error: unknown): boolean {
  return (
    error instanceof JetStreamApiError &&
    error.code === JetStreamApiCodes.ConsumerNotFound
  );
}

function waitForNatsRetry(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, natsRetryMilliseconds));
}
