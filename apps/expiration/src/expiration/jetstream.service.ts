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
import { connect, type NatsConnection } from '@nats-io/transport-node';
import {
  Injectable,
  Logger,
  OnApplicationShutdown,
  OnModuleInit,
} from '@nestjs/common';
import {
  expirationEventsStream,
  natsRetryMilliseconds,
  orderCreatedDurableName,
  orderCreatedSubject,
  ordersEventsStream,
} from './expiration.constants.js';

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
  ): Promise<void> {
    await this.requireClient().publish(subject, payload, { msgID: messageId });
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
