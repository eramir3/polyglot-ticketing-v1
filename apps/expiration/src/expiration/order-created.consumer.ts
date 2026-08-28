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
import { natsRetryMilliseconds } from './expiration.constants.js';
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
      if (error instanceof InvalidOrderCreatedEvent) {
        this.logger.error('terminal OrderCreated event', error.message);
        delivery.term(error.message);
        this.observe('terminal', started);
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
