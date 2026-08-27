import { BullModule } from '@nestjs/bullmq';
import { Module } from '@nestjs/common';
import { ExpirationProcessor } from './expiration.processor.js';
import { JetStreamExpirationPublisher } from './expiration.publisher.js';
import { OrderCreatedConsumer } from './order-created.consumer.js';
import { OrderCreatedHandler } from './order-created.handler.js';
import { ExpirationScheduler } from './expiration.scheduler.js';
import {
  EXPIRATION_EVENT_PUBLISHER,
  expirationQueueName,
} from './expiration.constants.js';
import { JetStreamService } from './jetstream.service.js';
import { redisConnectionOptions } from './redis.config.js';

@Module({
  imports: [
    BullModule.forRoot({ connection: redisConnectionOptions() }),
    BullModule.registerQueue({ name: expirationQueueName }),
  ],
  providers: [
    ExpirationScheduler,
    ExpirationProcessor,
    JetStreamService,
    JetStreamExpirationPublisher,
    {
      provide: EXPIRATION_EVENT_PUBLISHER,
      useExisting: JetStreamExpirationPublisher,
    },
    OrderCreatedHandler,
    OrderCreatedConsumer,
  ],
})
export class ExpirationModule {}
