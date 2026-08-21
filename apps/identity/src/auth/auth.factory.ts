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
      baseURL: requiredEnvironmentVariable('BETTER_AUTH_URL'),
      database: pool,
      emailAndPassword: {
        autoSignIn: false,
        enabled: true,
        requireEmailVerification: true,
      },
      emailVerification: {
        sendOnSignUp: true,
        sendVerificationEmail: async ({ user, token }) => {
          await emailSender.sendVerificationEmail({
            recipient: user.email,
            verificationUrl: createVerificationUrl(token),
          });
        },
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
