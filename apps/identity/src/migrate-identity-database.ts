import { createIdentityAuthContext } from './auth/auth.factory';

export async function migrateIdentityDatabase(): Promise<void> {
  const identityAuthContext = await createIdentityAuthContext();
  const { getMigrations } = await import('better-auth/db/migration');

  try {
    const { runMigrations } = await getMigrations(
      identityAuthContext.auth.options,
    );
    await runMigrations();
  } finally {
    await identityAuthContext.pool.end();
  }
}
