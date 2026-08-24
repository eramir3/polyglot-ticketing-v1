import { join } from 'node:path';
import { INestMicroservice } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { MicroserviceOptions, Transport } from '@nestjs/microservices';

// Shared by production startup and integration tests to keep NestJS configuration consistent.
export async function createIdentityMicroservice(
  grpcPort = Number(process.env.GRPC_PORT ?? 50051),
): Promise<INestMicroservice> {
  const { AppModule } = await import('./app.module.js');

  return NestFactory.createMicroservice<MicroserviceOptions>(AppModule, {
    transport: Transport.GRPC,
    options: {
      loader: {
        includeDirs: [
          join(process.cwd(), 'proto'),
          join(process.cwd(), 'proto-deps'),
        ],
      },
      package: 'identity.v1',
      protoPath: join(process.cwd(), 'proto/identity/v1/identity.proto'),
      url: `0.0.0.0:${grpcPort}`,
    },
  });
}
