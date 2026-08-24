import { Logger } from '@nestjs/common';
import { createApiGatewayApplication } from './app/app.bootstrap';

async function bootstrap() {
  const app = await createApiGatewayApplication();
  const globalPrefix = 'api';
  const port = Number(process.env.PORT ?? 3000);
  await app.listen(port);
  Logger.log(
    `API gateway is running on: http://localhost:${port}/${globalPrefix}`,
  );
}

bootstrap();
