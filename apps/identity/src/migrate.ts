import { migrateIdentityDatabase } from './migrate-identity-database';

void migrateIdentityDatabase().catch((error: unknown) => {
  console.error('Identity database migration failed.', error);
  process.exitCode = 1;
});
