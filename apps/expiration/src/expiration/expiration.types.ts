import type { JobsOptions } from 'bullmq';

export interface ExpirationJobData {
  orderId: string;
}

export interface ExpirationQueue {
  add(
    name: string,
    data: ExpirationJobData,
    options: JobsOptions,
  ): Promise<unknown>;
}

export interface ExpirationEventPublisher {
  publishExpirationComplete(orderId: string): Promise<void>;
}

export interface OrderCreatedDelivery {
  data: Uint8Array;
  ack(): void;
  nak(): void;
  term(reason?: string): void;
}
