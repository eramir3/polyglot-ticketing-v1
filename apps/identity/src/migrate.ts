import { createIdentityAuthContext } from './auth/auth.factory';

async function migrate() {
  const identityAuthContext = await createIdentityAuthContext();
  const { getMigrations } = await import('better-auth/db/migration');

  try {
    const { runMigrations } = await getMigrations(
      identityAuthContext.auth.options
    );
    await runMigrations();
  } finally {
    await identityAuthContext.pool.end();
  }
}

void migrate().catch((error: unknown) => {
  console.error('Identity database migration failed.', error);
  process.exitCode = 1;
});
