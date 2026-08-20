import { Pool } from 'pg';

export async function createIdentityAuthContext() {
  const pool = new Pool({
    connectionString: requiredEnvironmentVariable('DATABASE_URL'),
  });
  const { betterAuth } = await import('better-auth');

  return {
    auth: betterAuth({
      baseURL: requiredEnvironmentVariable('BETTER_AUTH_URL'),
      database: pool,
      emailAndPassword: {
        autoSignIn: false,
        enabled: true,
      },
      secret: requiredEnvironmentVariable('BETTER_AUTH_SECRET'),
    }),
    pool,
  };
}

export type IdentityAuthContext = Awaited<
  ReturnType<typeof createIdentityAuthContext>
>;

function requiredEnvironmentVariable(name: string): string {
  const value = process.env[name];

  if (!value) {
    throw new Error(`${name} must be configured.`);
  }

  return value;
}
