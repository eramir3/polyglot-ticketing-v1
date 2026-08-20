import { Inject, Injectable, OnApplicationShutdown } from '@nestjs/common';
import { IDENTITY_AUTH_CONTEXT } from './auth.constants';
import { IdentityAuthContext } from './auth.factory';

@Injectable()
export class IdentityAuthLifecycle implements OnApplicationShutdown {
  constructor(
    @Inject(IDENTITY_AUTH_CONTEXT)
    private readonly identityAuthContext: IdentityAuthContext
  ) {}

  async onApplicationShutdown(): Promise<void> {
    await this.identityAuthContext.pool.end();
  }
}
