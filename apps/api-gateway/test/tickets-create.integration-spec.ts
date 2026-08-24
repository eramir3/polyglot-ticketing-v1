import { ChildProcessWithoutNullStreams, spawn } from 'node:child_process';
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

describe('POST /api/tickets', () => {
  let apiGateway: INestApplication;
  let identity: INestMicroservice;
  let identityDatabase: StartedPostgreSqlContainer;
  let ticketsDatabase: StartedPostgreSqlContainer;
  let mailpit: StartedTestContainer;
  let ticketsProcess: ChildProcessWithoutNullStreams;
  let gatewayUrl: string;
  let identityDatabaseUrl: string;
  let ticketsDatabaseUrl: string;
  let sessionCookie: string;
  let userId: string;

  beforeAll(async () => {
    const [gatewayPort, identityGrpcPort, ticketsGrpcPort] = await Promise.all([
      getAvailablePort(),
      getAvailablePort(),
      getAvailablePort(),
    ]);

    [identityDatabase, ticketsDatabase] = await Promise.all([
      new PostgreSqlContainer('postgres:17-alpine')
        .withDatabase('identity')
        .withUsername('identity')
        .withPassword('identity-test-password')
        .start(),
      new PostgreSqlContainer('postgres:17-alpine')
        .withDatabase('tickets')
        .withUsername('tickets')
        .withPassword('tickets-test-password')
        .start(),
    ]);
    mailpit = await new GenericContainer('axllent/mailpit:v1.28')
      .withExposedPorts(1025)
      .withWaitStrategy(Wait.forListeningPorts())
      .start();

    gatewayUrl = `http://127.0.0.1:${gatewayPort}`;
    identityDatabaseUrl = identityDatabase.getConnectionUri();
    ticketsDatabaseUrl = ticketsDatabase.getConnectionUri();
    Object.assign(process.env, {
      BETTER_AUTH_SECRET: 'integration-test-secret-at-least-32-characters',
      BETTER_AUTH_URL: gatewayUrl,
      DATABASE_URL: identityDatabaseUrl,
      EMAIL_VERIFICATION_URL: `${gatewayUrl}/api/auth/verify-email`,
      IDENTITY_GRPC_URL: `127.0.0.1:${identityGrpcPort}`,
      SMTP_FROM: 'no-reply@polyglot-ticketing.test',
      SMTP_HOST: mailpit.getHost(),
      SMTP_PORT: mailpit.getMappedPort(1025).toString(),
      TICKETING_USER_APP_ORIGIN: 'http://localhost:3001',
      TICKETS_GRPC_URL: `127.0.0.1:${ticketsGrpcPort}`,
    });

    await migrateIdentityDatabase();
    await runTicketsMigration(withSslDisabled(ticketsDatabaseUrl));

    identity = await createIdentityMicroservice(identityGrpcPort);
    await identity.listen();
    ticketsProcess = startTicketsService(
      ticketsGrpcPort,
      withSslDisabled(ticketsDatabaseUrl),
    );
    await waitForPort(ticketsGrpcPort, ticketsProcess);

    apiGateway = await createApiGatewayApplication();
    await apiGateway.listen(gatewayPort, '127.0.0.1');

    ({ sessionCookie, userId } = await createAuthenticatedUser());
  });

  afterAll(async () => {
    await apiGateway?.close();
    await identity?.close();
    await stopTicketsService(ticketsProcess);
    await mailpit?.stop();
    await ticketsDatabase?.stop();
    await identityDatabase?.stop();
  });

  it('has a route handler listening to /api/tickets for POST requests', async () => {
    const response = await postTicket({ price: 10_000, title: 'Metallica' });

    expect(response.status).toBe(401);
    expect(response.body).toEqual({
      errors: [
        expect.objectContaining({
          code: 'UNAUTHENTICATED',
        }),
      ],
    });
  });

  it('can only be accessed if the user is signed in', async () => {
    const response = await postTicket({ price: 10_000, title: 'Metallica' });

    expect(response.status).toBe(401);
    expect(response.body).toEqual({
      errors: [
        expect.objectContaining({
          code: 'UNAUTHENTICATED',
        }),
      ],
    });
  });

  it('returns an error if an invalid title is provided', async () => {
    const response = await postTicket(
      { price: 10_000, title: '' },
      sessionCookie,
    );

    expect(response.status).toBe(400);
    expect(response.body).toEqual({
      errors: [
        expect.objectContaining({
          code: 'INVALID_TITLE',
          field: 'title',
        }),
      ],
    });
  });

  it('returns an error if an invalid price is provided', async () => {
    const response = await postTicket(
      { price: 0, title: 'Metallica' },
      sessionCookie,
    );

    expect(response.status).toBe(400);
    expect(response.body).toEqual({
      errors: [
        expect.objectContaining({
          code: 'INVALID_PRICE',
          field: 'price',
        }),
      ],
    });
  });

  it('creates a ticket with valid parameters', async () => {
    const response = await postTicket(
      { price: 10_000, title: 'Metallica' },
      sessionCookie,
    );

    expect(response.status).toBe(201);
    expect(response.body).toEqual({
      id: expect.any(String),
      price: 10_000,
      title: 'Metallica',
      userId,
    });

    const ticket = response.body as { id: string };
    const database = new Client({ connectionString: ticketsDatabaseUrl });
    await database.connect();
    try {
      const result = await database.query<{
        id: string;
        price: string;
        title: string;
        user_id: string;
      }>(
        `SELECT id::text, price::text, title, user_id
         FROM tickets
         WHERE id = $1`,
        [ticket.id],
      );

      expect(result.rows).toEqual([
        {
          id: ticket.id,
          price: '10000',
          title: 'Metallica',
          user_id: userId,
        },
      ]);
    } finally {
      await database.end();
    }
  });

  it('retrieves all tickets without authentication', async () => {
    const createdResponse = await postTicket(
      { price: 12_500, title: 'Iron Maiden' },
      sessionCookie,
    );
    expect(createdResponse.status).toBe(201);

    const response = await getJson('/api/tickets');

    expect(response.status).toBe(200);
    expect(response.body).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          price: 12_500,
          title: 'Iron Maiden',
          userId,
        }),
      ]),
    );
  });

  async function createAuthenticatedUser(): Promise<{
    sessionCookie: string;
    userId: string;
  }> {
    const email = `tickets-${Date.now()}@example.com`;
    const password = 'password123';
    const signupResponse = await postJson('/api/auth/signup', {
      email,
      name: 'Tickets Integration User',
      password,
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
      password,
    });
    expect(signinResponse.status).toBe(201);

    const setCookie = signinResponse.headers.get('set-cookie');
    if (!setCookie) {
      throw new Error('Expected signin to set the session cookie.');
    }

    return {
      sessionCookie: setCookie.split(';', 1)[0],
      userId: signup.userId,
    };
  }

  function postTicket(
    body: { title: string; price: number },
    cookie?: string,
  ): Promise<HttpResponse> {
    return postJson('/api/tickets', body, cookie);
  }

  async function postJson(
    path: string,
    body: Record<string, string | number>,
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

  async function getJson(path: string): Promise<HttpResponse> {
    const response = await fetch(`${gatewayUrl}${path}`);

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

/** Runs the tickets schema migrations against the isolated test database. */
function runTicketsMigration(databaseUrl: string): Promise<void> {
  return runGoCommand(['run', './cmd/migrate'], { DATABASE_URL: databaseUrl });
}

/** Starts the real Go tickets gRPC service in its own process group. */
function startTicketsService(
  grpcPort: number,
  databaseUrl: string,
): ChildProcessWithoutNullStreams {
  return spawn('go', ['run', './cmd/tickets'], {
    cwd: ticketsDirectory(),
    detached: true,
    env: {
      ...process.env,
      DATABASE_URL: databaseUrl,
      GRPC_PORT: grpcPort.toString(),
    },
    stdio: 'pipe',
  });
}

/** Executes a Go command and includes its captured output if it fails. */
async function runGoCommand(
  args: string[],
  environment: NodeJS.ProcessEnv,
): Promise<void> {
  const command = spawn('go', args, {
    cwd: ticketsDirectory(),
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

/** Waits for the tickets gRPC listener to accept TCP connections. */
async function waitForPort(
  port: number,
  ticketsProcess: ChildProcessWithoutNullStreams,
): Promise<void> {
  const deadline = Date.now() + 15_000;

  while (Date.now() < deadline) {
    if (ticketsProcess.exitCode !== null) {
      throw new Error('Tickets service exited before accepting gRPC requests.');
    }

    if (await canConnect(port)) {
      return;
    }

    await delay(100);
  }

  throw new Error('Tickets service did not start within 15 seconds.');
}

/** Reports whether a TCP connection can be established to the given local port. */
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

/** Stops the Go launcher and the compiled tickets child process it starts. */
async function stopTicketsService(
  ticketsProcess: ChildProcessWithoutNullStreams | undefined,
): Promise<void> {
  if (!ticketsProcess || ticketsProcess.exitCode !== null) {
    return;
  }

  const exited = new Promise<void>((resolve) => {
    ticketsProcess.once('exit', () => resolve());
  });
  terminateProcessGroup(ticketsProcess, 'SIGTERM');

  const stopped = await Promise.race([
    exited.then(() => true),
    delay(5_000).then(() => false),
  ]);
  if (!stopped) {
    terminateProcessGroup(ticketsProcess, 'SIGKILL');
    await exited;
  }
}

/** Sends a signal to the tickets process group, with a child-process fallback. */
function terminateProcessGroup(
  ticketsProcess: ChildProcessWithoutNullStreams,
  signal: NodeJS.Signals,
): void {
  if (ticketsProcess.pid === undefined) {
    return;
  }

  try {
    process.kill(-ticketsProcess.pid, signal);
  } catch {
    ticketsProcess.kill(signal);
  }
}

/** Collects subprocess output so failed Go commands include diagnostic logs. */
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

/** Resolves the tickets Go module directory from the workspace root. */
function ticketsDirectory(): string {
  return join(process.cwd(), 'apps/tickets');
}

/**
 * Disables SSL because Testcontainers Postgres does not serve TLS and the Go
 * golang-migrate Postgres driver otherwise attempts an SSL connection.
 */
function withSslDisabled(databaseUrl: string): string {
  const url = new URL(databaseUrl);
  url.searchParams.set('sslmode', 'disable');
  return url.toString();
}

/** Delays retry-based test setup or shutdown work. */
function delay(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

/** Reserves and releases an OS-assigned port for a local test server. */
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
