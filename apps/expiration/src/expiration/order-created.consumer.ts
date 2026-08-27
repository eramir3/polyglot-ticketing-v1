import {
  Injectable,
  Logger,
  OnApplicationBootstrap,
  OnApplicationShutdown,
} from '@nestjs/common';
import {
  InvalidOrderCreatedEvent,
  OrderCreatedHandler,
} from './order-created.handler.js';
import { JetStreamService } from './jetstream.service.js';
import { natsRetryMilliseconds } from './expiration.constants.js';
import { OrderCreatedDelivery } from './expiration.types.js';

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
    try {
      await this.handler.handle(delivery.data);
      delivery.ack();
    } catch (error) {
      if (error instanceof InvalidOrderCreatedEvent) {
        delivery.term(error.message);
        return;
      }

      delivery.nak();
    }
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
