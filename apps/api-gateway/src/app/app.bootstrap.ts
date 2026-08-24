import { INestApplication, ValidationPipe } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { ApiErrorFilter } from '../errors/api-error.filter';
import { createValidationApiError } from '../errors/validation-error';

// Shared by production startup and integration tests to keep NestJS configuration consistent.
export async function createApiGatewayApplication(): Promise<INestApplication> {
  const { AppModule } = await import('./app.module.js');
  const app = await NestFactory.create(AppModule);

  app.setGlobalPrefix('api');
  app.enableCors({
    credentials: true,
    origin: process.env.TICKETING_USER_APP_ORIGIN ?? 'http://localhost:3001',
  });
  app.useGlobalFilters(new ApiErrorFilter());
  app.useGlobalPipes(
    new ValidationPipe({
      exceptionFactory: createValidationApiError,
      forbidNonWhitelisted: true,
      stopAtFirstError: true,
      transform: true,
      whitelist: true,
    }),
  );

  return app;
}
