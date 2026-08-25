import { createServer } from 'node:net';
import { randomUUID } from 'node:crypto';
import { INestApplication, INestMicroservice } from '@nestjs/common';
import {
  PostgreSqlContainer,
  StartedPostgreSqlContainer,
} from '@testcontainers/postgresql';
import { Client } from 'pg';
import { GenericContainer, StartedTestContainer, Wait } from 'testcontainers';
import { createApiGatewayApplication } from '../src/app/app.bootstrap';
import { createIdentityMicroservice } from '../../identity/src/app/app.bootstrap';
import { migrateIdentityDatabase } from '../../identity/src/migrate-identity-database';

describe('authentication endpoints', () => {
  let apiGateway: INestApplication;
  let identity: INestMicroservice;
  let identityDatabase: StartedPostgreSqlContainer;
  let mailpit: StartedTestContainer;
  let gatewayUrl: string;
  let mailpitUrl: string;
  let databaseUrl: string;

  beforeAll(async () => {
    const [gatewayPort, grpcPort] = await Promise.all([
      getAvailablePort(),
      getAvailablePort(),
    ]);

    identityDatabase = await new PostgreSqlContainer('postgres:17-alpine')
      .withDatabase('identity')
      .withUsername('identity')
      .withPassword('identity-test-password')
      .start();
    mailpit = await new GenericContainer('axllent/mailpit:v1.28')
      .withExposedPorts(1025, 8025)
      .withWaitStrategy(Wait.forListeningPorts())
      .start();

    gatewayUrl = `http://127.0.0.1:${gatewayPort}`;
    mailpitUrl = `http://${mailpit.getHost()}:${mailpit.getMappedPort(8025)}`;
    databaseUrl = identityDatabase.getConnectionUri();
    Object.assign(process.env, {
      BETTER_AUTH_SECRET: 'integration-test-secret-at-least-32-characters',
      BETTER_AUTH_URL: gatewayUrl,
      DATABASE_URL: databaseUrl,
      EMAIL_VERIFICATION_URL: `${gatewayUrl}/api/auth/verify-email`,
      IDENTITY_GRPC_URL: `127.0.0.1:${grpcPort}`,
      SMTP_FROM: 'no-reply@polyglot-ticketing.test',
      SMTP_HOST: mailpit.getHost(),
      SMTP_PORT: mailpit.getMappedPort(1025).toString(),
      TICKETING_USER_APP_ORIGIN: 'http://localhost:3001',
    });

    await migrateIdentityDatabase();
    identity = await createIdentityMicroservice(grpcPort);
    await identity.listen();
    apiGateway = await createApiGatewayApplication();
    await apiGateway.listen(gatewayPort, '127.0.0.1');
  });

  afterAll(async () => {
    await apiGateway?.close();
    await identity?.close();
    await mailpit?.stop();
    await identityDatabase?.stop();
  });

  describe('POST /api/auth/signup', () => {
    it('creates the user and sends a verification email', async () => {
      const email = `signup-${Date.now()}@example.com`;
      const response = await postSignup({
        email,
        name: 'Integration Test User',
        password: 'password123',
      });

      expect(response.status).toBe(201);
      expect(response.body).toEqual({
        email,
        userId: expect.any(String),
      });

      const database = new Client({ connectionString: databaseUrl });
      await database.connect();
      try {
        const result = await database.query<{
          email: string;
          emailVerified: boolean;
          name: string;
        }>('SELECT email, name, "emailVerified" FROM "user" WHERE email = $1', [
          email,
        ]);

        expect(result.rows).toEqual([
          {
            email,
            emailVerified: false,
            name: 'Integration Test User',
          },
        ]);
      } finally {
        await database.end();
      }

      const emailMessage = await waitForEmail(email);
      expect(emailMessage).toContain(email);
      expect(emailMessage).toContain(
        `${gatewayUrl}/api/auth/verify-email?token=`,
      );
    });

    it('returns the standard error response when name is missing', async () => {
      const response = await postSignup({
        email: 'valid@example.com',
        password: 'password123',
      });

      expect(response.status).toBe(400);
      expect(response.body).toEqual({
        errors: [
          expect.objectContaining({
            code: 'INVALID_NAME',
            field: 'name',
          }),
        ],
      });
    });

    it.each([
      {
        body: {
          email: 'not-an-email',
          name: 'Integration Test User',
          password: 'password123',
        },
        code: 'INVALID_EMAIL',
        field: 'email',
        scenario: 'email is not valid',
      },
      {
        body: {
          email: 'valid@example.com',
          name: 'Integration Test User',
          password: 'short',
        },
        code: 'INVALID_PASSWORD',
        field: 'password',
        scenario: 'password is shorter than eight characters',
      },
      {
        body: {
          email: 'valid@example.com',
          name: 'Integration Test User',
          password: 'a'.repeat(129),
        },
        code: 'INVALID_PASSWORD',
        field: 'password',
        scenario: 'password is longer than 128 characters',
      },
    ])(
      'returns the standard error response when $scenario',
      async ({ body, code, field }) => {
        const response = await postSignup(body);

        expect(response.status).toBe(400);
        expect(response.body).toEqual({
          errors: [
            expect.objectContaining({
              code,
              field,
            }),
          ],
        });
      },
    );
  });

  describe('POST /api/auth/signin', () => {
    it('fails when an email that does not exist is supplied', async () => {
      const response = await postSignin({
        email: `missing-${randomUUID()}@example.com`,
        password: 'password123',
      });

      expect(response.status).toBe(401);
      expect(response.body).toEqual({
        errors: [
          expect.objectContaining({
            code: 'INVALID_CREDENTIALS',
          }),
        ],
      });
    });

    it('fails when an incorrect password is supplied', async () => {
      const { email } = await createVerifiedUser();
      const response = await postSignin({
        email,
        password: 'incorrect-password',
      });

      expect(response.status).toBe(401);
      expect(response.body).toEqual({
        errors: [
          expect.objectContaining({
            code: 'INVALID_CREDENTIALS',
          }),
        ],
      });
    });
  });

  async function postSignup(body: Record<string, string>): Promise<{
    status: number;
    body: unknown;
  }> {
    const response = await fetch(`${gatewayUrl}/api/auth/signup`, {
      body: JSON.stringify(body),
      headers: { 'content-type': 'application/json' },
      method: 'POST',
    });

    return { body: await response.json(), status: response.status };
  }

  async function postSignin(body: Record<string, string>): Promise<{
    status: number;
    body: unknown;
  }> {
    const response = await fetch(`${gatewayUrl}/api/auth/signin`, {
      body: JSON.stringify(body),
      headers: { 'content-type': 'application/json' },
      method: 'POST',
    });

    return { body: await response.json(), status: response.status };
  }

  async function createVerifiedUser(): Promise<{ email: string }> {
    const email = `signin-${randomUUID()}@example.com`;
    const signupResponse = await postSignup({
      email,
      name: 'Sign-in Integration User',
      password: 'password123',
    });
    if (signupResponse.status !== 201) {
      throw new Error(
        `Unable to create sign-in test user: ${signupResponse.status}`,
      );
    }

    const database = new Client({ connectionString: databaseUrl });
    await database.connect();
    try {
      await database.query(
        'UPDATE "user" SET "emailVerified" = TRUE WHERE email = $1',
        [email],
      );
    } finally {
      await database.end();
    }

    return { email };
  }

  async function waitForEmail(recipient: string): Promise<string> {
    const timeoutAt = Date.now() + 10_000;

    while (Date.now() < timeoutAt) {
      const response = await fetch(`${mailpitUrl}/api/v1/messages`);
      const messages = (await response.json()) as {
        messages: Array<{ ID: string }>;
      };
      const message = messages.messages.find((candidate) =>
        JSON.stringify(candidate).includes(recipient),
      );

      if (message) {
        const detailResponse = await fetch(
          `${mailpitUrl}/api/v1/message/${message.ID}`,
        );
        return detailResponse.text();
      }

      await new Promise((resolve) => setTimeout(resolve, 100));
    }

    throw new Error(`Mailpit did not receive an email for ${recipient}.`);
  }
});

async function getAvailablePort(): Promise<number> {
  const server = createServer();

  await new Promise<void>((resolve, reject) => {
    server.once('error', reject);
    server.listen(0, '127.0.0.1', resolve);
  });

  const address = server.address();
  await new Promise<void>((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()));
  });

  if (!address || typeof address === 'string') {
    throw new Error('Unable to allocate a local port for an integration test.');
  }

  return address.port;
}
