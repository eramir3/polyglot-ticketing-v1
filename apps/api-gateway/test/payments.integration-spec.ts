import { ChildProcessWithoutNullStreams, spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { createConnection, createServer } from 'node:net';
import { join } from 'node:path';
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

describe('payments endpoint', () => {
  let apiGateway: INestApplication;
  let identity: INestMicroservice;
  let identityDatabase: StartedPostgreSqlContainer;
  let paymentsDatabase: StartedPostgreSqlContainer;
  let mailpit: StartedTestContainer;
  let paymentsProcess: ChildProcessWithoutNullStreams;
  let gatewayUrl: string;
  let identityDatabaseUrl: string;
  let paymentsDatabaseUrl: string;
  let sessionCookie: string;
  let userId: string;

  beforeAll(async () => {
    const [gatewayPort, identityGrpcPort, paymentsGrpcPort] = await Promise.all(
      [getAvailablePort(), getAvailablePort(), getAvailablePort()],
    );
    [identityDatabase, paymentsDatabase] = await Promise.all([
      new PostgreSqlContainer('postgres:17-alpine')
        .withDatabase('identity')
        .withUsername('identity')
        .withPassword('identity-test-password')
        .start(),
      new PostgreSqlContainer('postgres:17-alpine')
        .withDatabase('payments')
        .withUsername('payments')
        .withPassword('payments-test-password')
        .start(),
    ]);
    mailpit = await new GenericContainer('axllent/mailpit:v1.28')
      .withExposedPorts(1025)
      .withWaitStrategy(Wait.forListeningPorts())
      .start();

    gatewayUrl = `http://127.0.0.1:${gatewayPort}`;
    identityDatabaseUrl = identityDatabase.getConnectionUri();
    paymentsDatabaseUrl = paymentsDatabase.getConnectionUri();
    Object.assign(process.env, {
      BETTER_AUTH_SECRET: 'integration-test-secret-at-least-32-characters',
      BETTER_AUTH_URL: gatewayUrl,
      DATABASE_URL: identityDatabaseUrl,
      EMAIL_VERIFICATION_URL: `${gatewayUrl}/api/auth/verify-email`,
      IDENTITY_GRPC_URL: `127.0.0.1:${identityGrpcPort}`,
      PAYMENTS_GRPC_URL: `127.0.0.1:${paymentsGrpcPort}`,
      SMTP_FROM: 'no-reply@polyglot-ticketing.test',
      SMTP_HOST: mailpit.getHost(),
      SMTP_PORT: mailpit.getMappedPort(1025).toString(),
      TICKETING_USER_APP_ORIGIN: 'http://localhost:3001',
    });

    await migrateIdentityDatabase();
    await runPaymentsMigration(withSslDisabled(paymentsDatabaseUrl));
    identity = await createIdentityMicroservice(identityGrpcPort);
    await identity.listen();
    paymentsProcess = startPaymentsService(
      paymentsGrpcPort,
      withSslDisabled(paymentsDatabaseUrl),
    );
    await waitForPort(paymentsGrpcPort, paymentsProcess);
    apiGateway = await createApiGatewayApplication();
    await apiGateway.listen(gatewayPort, '127.0.0.1');
    ({ sessionCookie, userId } = await createAuthenticatedUser());
  });

  afterAll(async () => {
    await apiGateway?.close();
    await identity?.close();
    await stopPaymentsService(paymentsProcess);
    await mailpit?.stop();
    await paymentsDatabase?.stop();
    await identityDatabase?.stop();
  });

  it('requires an authenticated user', async () => {
    const response = await postPayment({ orderId: randomUUID() });

    expect(response.status).toBe(401);
    expect(response.body).toEqual({
      errors: [expect.objectContaining({ code: 'UNAUTHENTICATED' })],
    });
  });

  it.each([{}, { orderId: 'not-a-uuid' }])(
    'validates orderId',
    async (body) => {
      const response = await postPayment(body, sessionCookie);

      expect(response.status).toBe(400);
      expect(response.body).toEqual({
        errors: [
          expect.objectContaining({
            code: 'INVALID_ARGUMENT',
            field: 'orderId',
          }),
        ],
      });
    },
  );

  it('creates one payment and returns it for a duplicate request', async () => {
    const orderId = await seedProjectedOrder(userId, 'Created');

    const created = await postPayment({ orderId }, sessionCookie);
    const repeated = await postPayment({ orderId }, sessionCookie);

    expect(created.status).toBe(201);
    expect(created.body).toEqual({ id: expect.any(String), orderId });
    expect(repeated.status).toBe(200);
    expect(repeated.body).toEqual(created.body);
  });

  it('returns 404 when the order does not exist', async () => {
    const response = await postPayment({ orderId: randomUUID() }, sessionCookie);

    expect(response.status).toBe(404);
    expect(response.body).toEqual({
      errors: [expect.objectContaining({ code: 'NOT_FOUND' })],
    });
  });

  it('hides an order owned by another user', async () => {
    const response = await postPayment(
      { orderId: await seedProjectedOrder('another-user', 'Created') },
      sessionCookie,
    );

    expect(response.status).toBe(404);
    expect(response.body).toEqual({
      errors: [expect.objectContaining({ code: 'NOT_FOUND' })],
    });
  });

  it('rejects a canceled projected order', async () => {
    const response = await postPayment(
      { orderId: await seedProjectedOrder(userId, 'Canceled') },
      sessionCookie,
    );

    expect(response.status).toBe(409);
    expect(response.body).toEqual({
      errors: [
        expect.objectContaining({
          code: 'ALREADY_EXISTS',
          message: 'Order cannot be paid.',
        }),
      ],
    });
  });

  async function createAuthenticatedUser(): Promise<{
    sessionCookie: string;
    userId: string;
  }> {
    const email = `payments-${randomUUID()}@example.com`;
    const signupResponse = await postJson('/api/auth/signup', {
      email,
      name: 'Payments Integration User',
      password: 'password123',
    });
    expect(signupResponse.status).toBe(201);
    const signup = signupResponse.body as { userId: string };

    const database = new Client({ connectionString: identityDatabaseUrl });
    await database.connect();
    try {
      await database.query(
        'UPDATE "user" SET "emailVerified" = true WHERE id = $1',
        [signup.userId],
      );
    } finally {
      await database.end();
    }

    const signinResponse = await postJson('/api/auth/signin', {
      email,
      password: 'password123',
    });
    expect(signinResponse.status).toBe(201);
    const setCookie = signinResponse.headers.get('set-cookie');
    if (!setCookie) {
      throw new Error('Expected signin to set the session cookie.');
    }
    return { sessionCookie: setCookie.split(';', 1)[0], userId: signup.userId };
  }

  async function seedProjectedOrder(
    ownerId: string,
    status: 'Canceled' | 'Created',
  ): Promise<string> {
    const orderId = randomUUID();
    const database = new Client({ connectionString: paymentsDatabaseUrl });
    await database.connect();
    try {
      await database.query(
        `INSERT INTO orders (id, aggregate_version, user_id, price, status)
         VALUES ($1, $2, $3, $4, $5)`,
        [orderId, 0, ownerId, 10_000, status],
      );
    } finally {
      await database.end();
    }
    return orderId;
  }

  function postPayment(
    body: Record<string, string | undefined>,
    cookie?: string,
  ): Promise<HttpResponse> {
    return postJson('/api/payments', body, cookie);
  }

  async function postJson(
    path: string,
    body: Record<string, string | undefined>,
    cookie?: string,
  ): Promise<HttpResponse> {
    const response = await fetch(`${gatewayUrl}${path}`, {
      body: JSON.stringify(body),
      headers: {
        'content-type': 'application/json',
        ...(cookie === undefined ? {} : { cookie }),
      },
      method: 'POST',
    });
    return {
      body: await response.json(),
      headers: response.headers,
      status: response.status,
    };
  }
});

interface HttpResponse {
  body: unknown;
  headers: Headers;
  status: number;
}

function runPaymentsMigration(databaseUrl: string): Promise<void> {
  return runGoCommand(['run', './cmd/migrate'], { DATABASE_URL: databaseUrl });
}

function startPaymentsService(
  grpcPort: number,
  databaseUrl: string,
): ChildProcessWithoutNullStreams {
  return spawn('go', ['run', './cmd/payments'], {
    cwd: paymentsDirectory(),
    detached: true,
    env: {
      ...process.env,
      DATABASE_URL: databaseUrl,
      GRPC_PORT: grpcPort.toString(),
      NATS_URL: 'nats://127.0.0.1:1',
    },
    stdio: 'pipe',
  });
}

async function runGoCommand(
  args: string[],
  environment: NodeJS.ProcessEnv,
): Promise<void> {
  const command = spawn('go', args, {
    cwd: paymentsDirectory(),
    env: { ...process.env, ...environment },
    stdio: 'pipe',
  });
  const output = collectProcessOutput(command);
  await new Promise<void>((resolve, reject) => {
    command.once('error', reject);
    command.once('exit', (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(
        new Error(
          `go ${args.join(' ')} failed with exit code ${code}: ${output()}`,
        ),
      );
    });
  });
}

async function waitForPort(
  port: number,
  paymentsProcess: ChildProcessWithoutNullStreams,
): Promise<void> {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    if (paymentsProcess.exitCode !== null) {
      throw new Error(
        'Payments service exited before accepting gRPC requests.',
      );
    }
    if (await canConnect(port)) {
      return;
    }
    await delay(100);
  }
  throw new Error('Payments service did not start within 15 seconds.');
}

function canConnect(port: number): Promise<boolean> {
  return new Promise((resolve) => {
    const connection = createConnection({ host: '127.0.0.1', port });
    connection.once('connect', () => {
      connection.end();
      resolve(true);
    });
    connection.once('error', () => resolve(false));
  });
}

async function stopPaymentsService(
  paymentsProcess: ChildProcessWithoutNullStreams | undefined,
): Promise<void> {
  if (!paymentsProcess || paymentsProcess.exitCode !== null) {
    return;
  }
  const exited = new Promise<void>((resolve) => {
    paymentsProcess.once('exit', () => resolve());
  });
  terminateProcessGroup(paymentsProcess, 'SIGTERM');
  const stopped = await Promise.race([
    exited.then(() => true),
    delay(5_000).then(() => false),
  ]);
  if (!stopped) {
    terminateProcessGroup(paymentsProcess, 'SIGKILL');
    await exited;
  }
}

function terminateProcessGroup(
  childProcess: ChildProcessWithoutNullStreams,
  signal: NodeJS.Signals,
): void {
  if (childProcess.pid === undefined) {
    return;
  }
  try {
    globalThis.process.kill(-childProcess.pid, signal);
  } catch {
    childProcess.kill(signal);
  }
}

function collectProcessOutput(
  process: ChildProcessWithoutNullStreams,
): () => string {
  let output = '';
  process.stdout.on('data', (chunk: Buffer) => {
    output += chunk.toString();
  });
  process.stderr.on('data', (chunk: Buffer) => {
    output += chunk.toString();
  });
  return () => output;
}

function getAvailablePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer();
    server.once('error', reject);
    server.listen(0, '127.0.0.1', () => {
      const address = server.address();
      if (address === null || typeof address === 'string') {
        reject(new Error('Unable to allocate a TCP port.'));
        return;
      }
      server.close((error) => (error ? reject(error) : resolve(address.port)));
    });
  });
}

function withSslDisabled(databaseUrl: string): string {
  return databaseUrl.includes('?')
    ? `${databaseUrl}&sslmode=disable`
    : `${databaseUrl}?sslmode=disable`;
}

function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

function paymentsDirectory(): string {
  return join(process.cwd(), 'apps/payments');
}
