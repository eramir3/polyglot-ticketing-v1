import {
  Injectable,
  Logger,
  OnApplicationBootstrap,
  OnApplicationShutdown,
  Optional,
} from '@nestjs/common';
import {
  InvalidOrderCreatedEvent,
  OrderCreatedHandler,
} from './order-created.handler.js';
import { JetStreamService } from './jetstream.service.js';
import {
  natsRetryMilliseconds,
  orderCreatedMaxRetries,
} from './expiration.constants.js';
import { OrderCreatedDelivery } from './expiration.types.js';
import { ExpirationMetrics } from '../observability/prometheus.js';

@Injectable()
export class OrderCreatedConsumer
  implements OnApplicationBootstrap, OnApplicationShutdown
{
  private readonly logger = new Logger(OrderCreatedConsumer.name);
  private processing?: Promise<void>;
  private stopping = false;
  private stopMessages?: () => void;

  constructor(
    private readonly handler: OrderCreatedHandler,
    private readonly jetStream: JetStreamService,
    @Optional() private readonly metrics?: ExpirationMetrics,
  ) {}

  onApplicationBootstrap(): void {
    this.processing = this.consume();
  }

  async onApplicationShutdown(): Promise<void> {
    this.stopping = true;
    this.stopMessages?.();
    await this.processing;
  }

  async handleDelivery(delivery: OrderCreatedDelivery): Promise<void> {
    const started = performance.now();
    try {
      await this.handler.handle(delivery.data);
      delivery.ack();
      this.observe('success', started);
    } catch (error) {
      const failureClass = this.failureClass(error);
      if (
        failureClass === 'invalid' ||
        delivery.info.deliveryCount > orderCreatedMaxRetries
      ) {
        if (await this.park(delivery, failureClass, error)) {
          this.observe('dead_lettered', started);
          return;
        }
        this.observe('dead_letter_publish_failed', started);
        return;
      }

      this.logger.warn(
        'order-created handling failed; event will be retried',
        error,
      );
      delivery.nak();
      this.observe('retry', started);
    }
  }

  private async park(
    delivery: OrderCreatedDelivery,
    failureClass: string,
    failure: unknown,
  ): Promise<boolean> {
    try {
      await this.jetStream.parkOrderCreated(delivery, failureClass, failure);
    } catch (error) {
      this.logger.error(
        'failed to park OrderCreated event in dead letter queue; event will be retried',
        error,
      );
      delivery.nak();
      return false;
    }

    delivery.ack();
    this.logger.warn('OrderCreated event parked in dead letter queue', {
      deliveryCount: delivery.info.deliveryCount,
      error: failure instanceof Error ? failure.message : String(failure),
      failureClass,
      streamSequence: delivery.info.streamSequence,
      subject: delivery.subject,
    });
    return true;
  }

  private failureClass(error: unknown): string {
    if (error instanceof InvalidOrderCreatedEvent) {
      return 'invalid';
    }
    return 'retryable';
  }

  private observe(outcome: string, started: number): void {
    this.metrics?.observe(
      'jetstream_consumer',
      'order_created',
      outcome,
      (performance.now() - started) / 1_000,
    );
  }

  private async consume(): Promise<void> {
    while (!this.stopping) {
      try {
        const consumer = await this.jetStream.orderCreatedConsumer();
        const messages = await consumer.consume();
        this.stopMessages = () => messages.stop();

        for await (const message of messages) {
          await this.handleDelivery(message);
        }
      } catch (error) {
        if (!this.stopping) {
          this.logger.warn(
            'order-created consumer stopped; reconnecting',
            error,
          );
          await waitForRetry();
        }
      } finally {
        this.stopMessages = undefined;
      }
    }
  }
}

function waitForRetry(): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, natsRetryMilliseconds));
}
