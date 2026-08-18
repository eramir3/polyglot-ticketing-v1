import { Injectable } from '@nestjs/common';

@Injectable()
export class AppService {
  getData(): { service: string; status: string } {
    return { service: 'api-gateway', status: 'ok' };
  }
}
