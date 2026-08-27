import { fromBinary } from '@bufbuild/protobuf';
import { ExpirationCompleteSchema } from '../../../../protogen/ts/expiration/v1/events_pb.js';
import { expirationCompleteSubject } from './expiration.constants.js';
import { JetStreamExpirationPublisher } from './expiration.publisher.js';
import { JetStreamService } from './jetstream.service.js';

describe('JetStreamExpirationPublisher', () => {
  it('publishes an ExpirationComplete protobuf event with a stable message ID', async () => {
    const jetStream = new FakeJetStreamService();
    const publisher = new JetStreamExpirationPublisher(jetStream as never);

    await publisher.publishExpirationComplete('order-1');

    expect(jetStream.calls).toHaveLength(1);
    const published = jetStream.calls[0]!;
    expect(published.subject).toBe(expirationCompleteSubject);
    expect(published.messageId).toBe('order-1');

    const event = fromBinary(ExpirationCompleteSchema, published.payload);
    expect(event.eventId).toBe('order-1');
    expect(event.orderId).toBe('order-1');
    expect(event.occurredAt).toBeDefined();
  });
});

class FakeJetStreamService {
  calls: Array<{ messageId: string; payload: Uint8Array; subject: string }> =
    [];

  async publish(
    subject: string,
    payload: Uint8Array,
    messageId: string,
  ): Promise<void> {
    this.calls.push({ messageId, payload, subject });
  }
}
