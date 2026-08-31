import type { JobsOptions } from 'bullmq';
import type { MsgHdrs } from '@nats-io/transport-node';

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
  headers?: MsgHdrs;
  info: {
    deliveryCount: number;
    stream: string;
    streamSequence: number;
  };
  subject: string;
  ack(): void;
  nak(): void;
}
