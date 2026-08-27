import { create, toBinary } from '@bufbuild/protobuf';
import { timestampNow } from '@bufbuild/protobuf/wkt';
import { Injectable } from '@nestjs/common';
import { ExpirationCompleteSchema } from '../../../../protogen/ts/expiration/v1/events_pb.js';
import { expirationCompleteSubject } from './expiration.constants.js';
import { ExpirationEventPublisher } from './expiration.types.js';
import { JetStreamService } from './jetstream.service.js';

@Injectable()
export class JetStreamExpirationPublisher implements ExpirationEventPublisher {
  constructor(private readonly jetStream: JetStreamService) {}

  async publishExpirationComplete(orderId: string): Promise<void> {
    const payload = toBinary(
      ExpirationCompleteSchema,
      create(ExpirationCompleteSchema, {
        eventId: orderId,
        occurredAt: timestampNow(),
        orderId,
      }),
    );

    await this.jetStream.publish(expirationCompleteSubject, payload, orderId);
  }
}
