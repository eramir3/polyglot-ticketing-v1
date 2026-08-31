import { Logger } from '@nestjs/common';
import { JetStreamService } from './jetstream.service.js';

async function main(): Promise<void> {
  const sequence = requiredSequence('DLQ_SEQUENCE');
  const jetStream = new JetStreamService();
  await jetStream.onModuleInit();
  try {
    const sourceSequence = await jetStream.replayExpirationDeadLetter(sequence);
    process.stdout.write(
      `replayed Expiration DLQ sequence ${sequence} as source stream sequence ${sourceSequence}\n`,
    );
  } finally {
    await jetStream.onApplicationShutdown();
  }
}

function requiredSequence(name: string): number {
  const value = process.env[name];
  if (value === undefined || value.trim() === '') {
    throw new Error(`${name} is required`);
  }
  const sequence = Number(value);
  if (!Number.isSafeInteger(sequence) || sequence <= 0) {
    throw new Error(`${name} must be a positive integer`);
  }
  return sequence;
}

void main().catch((error: unknown) => {
  Logger.error(error, undefined, 'ExpirationDLQReplay');
  process.exitCode = 1;
});
