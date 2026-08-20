import { Logger, ValidationPipe } from '@nestjs/common';
import { NestFactory } from '@nestjs/core';
import { AppModule } from './app/app.module';
import { ApiErrorFilter } from './errors/api-error.filter';
import { createValidationApiError } from './errors/validation-error';

async function bootstrap() {
  const app = await NestFactory.create(AppModule);
  const globalPrefix = 'api';
  app.setGlobalPrefix(globalPrefix);
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
  const port = Number(process.env.PORT ?? 3000);
  await app.listen(port);
  Logger.log(
    `API gateway is running on: http://localhost:${port}/${globalPrefix}`,
  );
}

bootstrap();
