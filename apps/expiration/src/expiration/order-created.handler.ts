import { fromBinary } from '@bufbuild/protobuf';
import { timestampDate } from '@bufbuild/protobuf/wkt';
import { Injectable } from '@nestjs/common';
import { OrderCreatedSchema } from '../../../../protogen/ts/orders/v1/events_pb.js';
import { OrderStatus } from '../../../../protogen/ts/orders/v1/orders_pb.js';
import { ExpirationScheduler } from './expiration.scheduler.js';

export class InvalidOrderCreatedEvent extends Error {
  constructor() {
    super('OrderCreated event is invalid.');
  }
}

@Injectable()
export class OrderCreatedHandler {
  constructor(private readonly scheduler: ExpirationScheduler) {}

  async handle(payload: Uint8Array): Promise<void> {
    let event;
    try {
      event = fromBinary(OrderCreatedSchema, payload);
    } catch {
      throw new InvalidOrderCreatedEvent();
    }

    if (
      event.orderId.trim() === '' ||
      event.orderStatus !== OrderStatus.CREATED ||
      event.expiresAt === undefined
    ) {
      throw new InvalidOrderCreatedEvent();
    }

    const expiresAt = timestampDate(event.expiresAt);
    if (Number.isNaN(expiresAt.getTime())) {
      throw new InvalidOrderCreatedEvent();
    }

    await this.scheduler.schedule(event.orderId, expiresAt);
  }
}
