import { Pool } from 'pg';
import { createEmailSender, createVerificationUrl } from './email.sender';

export async function createIdentityAuthContext() {
  const pool = new Pool({
    connectionString: requiredEnvironmentVariable('DATABASE_URL'),
  });
  const { betterAuth } = await import('better-auth');
  const emailSender = createEmailSender();

  return {
    auth: betterAuth({
      advanced: {
        cookies: {
          session_token: {
            name: 'better-auth.session_token',
          },
        },
      },
      baseURL: requiredEnvironmentVariable('BETTER_AUTH_URL'),
      database: pool,
      emailAndPassword: {
        autoSignIn: false,
        enabled: true,
        requireEmailVerification: true,
      },
      emailVerification: {
        sendOnSignIn: false,
        sendOnSignUp: true,
        sendVerificationEmail: async ({ user, token }) => {
          await emailSender.sendVerificationEmail({
            recipient: user.email,
            verificationUrl: createVerificationUrl(token),
          });
        },
      },
      secret: requiredEnvironmentVariable('BETTER_AUTH_SECRET'),
      trustedOrigins: [
        process.env.TICKETING_USER_APP_ORIGIN ?? 'http://localhost:3001',
      ],
    }),
    getSessionExpiry: async (
      sessionToken: string,
      userId: string,
    ): Promise<Date | null> => {
      const result = await pool.query<{ expiresAt: Date }>(
        'SELECT "expiresAt" FROM "session" WHERE token = $1 AND "userId" = $2',
        [sessionToken, userId],
      );

      return result.rows[0]?.expiresAt ?? null;
    },
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
