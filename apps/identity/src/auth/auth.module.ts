import { Module } from '@nestjs/common';
import { IDENTITY_AUTH_CONTEXT } from './auth.constants';
import { createIdentityAuthContext } from './auth.factory';
import { IdentityAuthLifecycle } from './auth.lifecycle';

@Module({
  providers: [
    {
      provide: IDENTITY_AUTH_CONTEXT,
      useFactory: createIdentityAuthContext,
    },
    IdentityAuthLifecycle,
  ],
  exports: [IDENTITY_AUTH_CONTEXT],
})
export class IdentityAuthModule {}
