import { ChildProcessWithoutNullStreams, spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { createConnection, createServer } from 'node:net';
import { join } from 'node:path';
import { INestApplication, INestMicroservice } from '@nestjs/common';
import { fromBinary } from '@bufbuild/protobuf';
import {
  PostgreSqlContainer,
  StartedPostgreSqlContainer,
} from '@testcontainers/postgresql';
import { Client } from 'pg';
import { GenericContainer, StartedTestContainer, Wait } from 'testcontainers';
import { createApiGatewayApplication } from '../src/app/app.bootstrap';
import { createIdentityMicroservice } from '../../identity/src/app/app.bootstrap';
import { migrateIdentityDatabase } from '../../identity/src/migrate-identity-database';
import {
  OrderCanceledSchema,
  OrderCreatedSchema,
} from '../../../protogen/ts/orders/v1/events_pb.js';
import { OrderStatus } from '../../../protogen/ts/orders/v1/orders_pb.js';

describe('orders endpoints', () => {
  let apiGateway: INestApplication;
  let identity: INestMicroservice;
  let identityDatabase: StartedPostgreSqlContainer;
  let ordersDatabase: StartedPostgreSqlContainer;
  let mailpit: StartedTestContainer;
  let ordersProcess: ChildProcessWithoutNullStreams;
  let gatewayUrl: string;
  let identityDatabaseUrl: string;
  let ordersDatabaseUrl: string;
  let sessionCookie: string;
  let userId: string;
  let anotherUser: AuthenticatedUser;

  beforeAll(async () => {
    const [gatewayPort, identityGrpcPort, ordersGrpcPort] = await Promise.all([
      getAvailablePort(),
      getAvailablePort(),
      getAvailablePort(),
    ]);
    [identityDatabase, ordersDatabase] = await Promise.all([
      new PostgreSqlContainer('postgres:17-alpine')
        .withDatabase('identity')
        .withUsername('identity')
        .withPassword('identity-test-password')
        .start(),
      new PostgreSqlContainer('postgres:17-alpine')
        .withDatabase('orders')
        .withUsername('orders')
        .withPassword('orders-test-password')
        .start(),
    ]);
    mailpit = await new GenericContainer('axllent/mailpit:v1.28')
      .withExposedPorts(1025)
      .withWaitStrategy(Wait.forListeningPorts())
      .start();

    gatewayUrl = `http://127.0.0.1:${gatewayPort}`;
    identityDatabaseUrl = identityDatabase.getConnectionUri();
    ordersDatabaseUrl = ordersDatabase.getConnectionUri();
    Object.assign(process.env, {
      BETTER_AUTH_SECRET: 'integration-test-secret-at-least-32-characters',
      BETTER_AUTH_URL: gatewayUrl,
      DATABASE_URL: identityDatabaseUrl,
      EMAIL_VERIFICATION_URL: `${gatewayUrl}/api/auth/verify-email`,
      IDENTITY_GRPC_URL: `127.0.0.1:${identityGrpcPort}`,
      ORDERS_GRPC_URL: `127.0.0.1:${ordersGrpcPort}`,
      SMTP_FROM: 'no-reply@polyglot-ticketing.test',
      SMTP_HOST: mailpit.getHost(),
      SMTP_PORT: mailpit.getMappedPort(1025).toString(),
      TICKETING_USER_APP_ORIGIN: 'http://localhost:3001',
    });

    await migrateIdentityDatabase();
    await runOrdersMigration(withSslDisabled(ordersDatabaseUrl));
    identity = await createIdentityMicroservice(identityGrpcPort);
    await identity.listen();
    ordersProcess = startOrdersService(
      ordersGrpcPort,
      withSslDisabled(ordersDatabaseUrl),
    );
    await waitForPort(ordersGrpcPort, ordersProcess);
    apiGateway = await createApiGatewayApplication();
    await apiGateway.listen(gatewayPort, '127.0.0.1');
    ({ sessionCookie, userId } = await createAuthenticatedUser());
    anotherUser = await createAuthenticatedUser();
  });

  afterAll(async () => {
    await apiGateway?.close();
    await identity?.close();
    await stopOrdersService(ordersProcess);
    await mailpit?.stop();
    await ordersDatabase?.stop();
    await identityDatabase?.stop();
  });

  describe('create orders', () => {
    it('requires an authenticated user', async () => {
      const response = await postOrder({ ticketId: randomUUID() });

      expect(response.status).toBe(401);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'UNAUTHENTICATED' })],
      });
    });

    it('rejects an invalid ticket ID', async () => {
      const response = await postOrder(
        { ticketId: 'not-a-uuid' },
        sessionCookie,
      );

      expect(response.status).toBe(400);
      expect(response.body).toEqual({
        errors: [
          expect.objectContaining({
            code: 'INVALID_ARGUMENT',
            field: 'ticketId',
          }),
        ],
      });
    });

    it('returns 404 until the ticket exists in the Orders projection', async () => {
      const response = await postOrder(
        { ticketId: randomUUID() },
        sessionCookie,
      );

      expect(response.status).toBe(404);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'NOT_FOUND' })],
      });
    });

    it('creates an order for a projected ticket', async () => {
      const ticketId = await seedProjectedTicket();
      const beforeCreation = Date.now();

      const response = await postOrder({ ticketId }, sessionCookie);

      expect(response.status).toBe(201);
      expect(response.body).toEqual({
        expiresAt: expect.any(String),
        id: expect.any(String),
        status: 'Created',
        ticketId,
        userId,
      });
      const expiresAt = Date.parse(
        (response.body as { expiresAt: string }).expiresAt,
      );
      expect(expiresAt).toBeGreaterThanOrEqual(beforeCreation + 14 * 60_000);
      expect(expiresAt).toBeLessThanOrEqual(Date.now() + 16 * 60_000);
    });

    it('writes one OrderCreated event for the first reservation', async () => {
      const ticketId = await seedProjectedTicket();
      const creationCountBefore = await countCreatedOrderEvents();

      const firstCreation = await postOrder({ ticketId }, sessionCookie);
      const eventsAfterFirstCreation = await createdOrderEvents();
      const secondCreation = await postOrder({ ticketId }, sessionCookie);
      const creationCountAfterRetry = await countCreatedOrderEvents();

      expect(firstCreation.status).toBe(201);
      expect(eventsAfterFirstCreation).toHaveLength(creationCountBefore + 1);
      const eventRow =
        eventsAfterFirstCreation[eventsAfterFirstCreation.length - 1];
      expect(eventRow).toBeDefined();
      const event = fromBinary(OrderCreatedSchema, eventRow!.payload);
      expect(event).toMatchObject({
        eventId: eventRow!.event_id,
        orderId: (firstCreation.body as OrderResponse).id,
        orderStatus: OrderStatus.CREATED,
        userId,
        ticket: { id: ticketId, price: 10_000n },
      });
      expect(event.occurredAt).toBeDefined();
      expect(event.expiresAt).toBeDefined();
      expect(secondCreation.status).toBe(200);
      expect(creationCountAfterRetry).toBe(creationCountBefore + 1);
    });

    it('returns the existing order for a same-user retry without extending it', async () => {
      const ticketId = await seedProjectedTicket();

      const first = await postOrder({ ticketId }, sessionCookie);
      const second = await postOrder({ ticketId }, sessionCookie);

      expect(first.status).toBe(201);
      expect(second.status).toBe(200);
      expect(second.body).toEqual(first.body);
    });

    it('blocks another user while a ticket has an active Created order', async () => {
      const ticketId = await seedProjectedTicket();
      await expect(
        postOrder({ ticketId }, sessionCookie),
      ).resolves.toMatchObject({
        status: 201,
      });

      const response = await postOrder({ ticketId }, anotherUser.sessionCookie);

      expect(response.status).toBe(409);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'ALREADY_EXISTS' })],
      });
    });

    it('blocks another user while a ticket has an active AwaitingPayment order', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);
      await setOrderState(
        (created.body as OrderResponse).id,
        'AwaitingPayment',
      );

      const response = await postOrder({ ticketId }, anotherUser.sessionCookie);

      expect(response.status).toBe(409);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'ALREADY_EXISTS' })],
      });
    });

    it('allows another user after the reservation expires', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);
      await setOrderState((created.body as OrderResponse).id, 'Created', true);

      const response = await postOrder({ ticketId }, anotherUser.sessionCookie);

      expect(response.status).toBe(201);
      expect(response.body).toEqual(
        expect.objectContaining({ ticketId, userId: anotherUser.userId }),
      );
    });

    it('allows another user after the reservation is canceled', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);
      await setOrderState((created.body as OrderResponse).id, 'Canceled');

      const response = await postOrder({ ticketId }, anotherUser.sessionCookie);

      expect(response.status).toBe(201);
    });

    it('blocks all users after the ticket order is complete', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);
      await setOrderState((created.body as OrderResponse).id, 'Complete', true);

      const response = await postOrder({ ticketId }, anotherUser.sessionCookie);

      expect(response.status).toBe(409);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'ALREADY_EXISTS' })],
      });
    });

    it('allows only one of two concurrent users to reserve a ticket', async () => {
      const ticketId = await seedProjectedTicket();

      const responses = await Promise.all([
        postOrder({ ticketId }, sessionCookie),
        postOrder({ ticketId }, anotherUser.sessionCookie),
      ]);

      expect(responses.map(({ status }) => status).sort()).toEqual([201, 409]);
    });
  });

  describe('read orders', () => {
    it('requires an authenticated user', async () => {
      const response = await getOrders();

      expect(response.status).toBe(401);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'UNAUTHENTICATED' })],
      });
    });

    it('returns an empty list when the authenticated user has no orders', async () => {
      const userWithoutOrders = await createAuthenticatedUser();

      const response = await getOrders(userWithoutOrders.sessionCookie);

      expect(response.status).toBe(200);
      expect(response.body).toEqual([]);
    });

    it('lists only the orders belonging to the authenticated user', async () => {
      const listingUser = await createAuthenticatedUser();
      const earlierTicketId = await seedProjectedTicket();
      const laterTicketId = await seedProjectedTicket();
      const otherTicketId = await seedProjectedTicket();

      const earlierOrder = await postOrder(
        { ticketId: earlierTicketId },
        listingUser.sessionCookie,
      );
      const otherOrder = await postOrder(
        { ticketId: otherTicketId },
        anotherUser.sessionCookie,
      );
      const laterOrder = await postOrder(
        { ticketId: laterTicketId },
        listingUser.sessionCookie,
      );
      await setOrderExpiration(
        (earlierOrder.body as OrderResponse).id,
        5 * 60_000,
      );
      await setOrderExpiration(
        (laterOrder.body as OrderResponse).id,
        30 * 60_000,
      );
      const response = await getOrders(listingUser.sessionCookie);

      expect(earlierOrder.status).toBe(201);
      expect(otherOrder.status).toBe(201);
      expect(laterOrder.status).toBe(201);
      expect(response.status).toBe(200);
      expect(response.body).toEqual([
        expect.objectContaining({
          expiresAt: expect.any(String),
          id: (laterOrder.body as OrderResponse).id,
          status: 'Created',
          ticketId: laterTicketId,
          userId: listingUser.userId,
        }),
        expect.objectContaining({
          expiresAt: expect.any(String),
          id: (earlierOrder.body as OrderResponse).id,
          status: 'Created',
          ticketId: earlierTicketId,
          userId: listingUser.userId,
        }),
      ]);
    });

    it('requires an authenticated user to retrieve an order', async () => {
      const response = await getOrder(randomUUID());

      expect(response.status).toBe(401);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'UNAUTHENTICATED' })],
      });
    });

    it('retrieves an order belonging to the authenticated user', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);

      const response = await getOrder(
        (created.body as OrderResponse).id,
        sessionCookie,
      );

      expect(created.status).toBe(201);
      expect(response.status).toBe(200);
      expect(response.body).toEqual(created.body);
    });

    it('returns 404 when the order does not exist', async () => {
      const response = await getOrder(randomUUID(), sessionCookie);

      expect(response.status).toBe(404);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'NOT_FOUND' })],
      });
    });

    it('returns 404 when another user owns the order', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);

      const response = await getOrder(
        (created.body as OrderResponse).id,
        anotherUser.sessionCookie,
      );

      expect(created.status).toBe(201);
      expect(response.status).toBe(404);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'NOT_FOUND' })],
      });
    });

    it('rejects an invalid order ID', async () => {
      const response = await getOrder('not-a-uuid', sessionCookie);

      expect(response.status).toBe(400);
      expect(response.body).toEqual({
        errors: [
          expect.objectContaining({
            code: 'INVALID_ARGUMENT',
            field: 'orderId',
          }),
        ],
      });
    });
  });

  describe('delete orders', () => {
    it('requires an authenticated user', async () => {
      const response = await deleteOrder(randomUUID());

      expect(response.status).toBe(401);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'UNAUTHENTICATED' })],
      });
    });

    it('cancels an active order belonging to the authenticated user', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);

      const response = await deleteOrder(
        (created.body as OrderResponse).id,
        sessionCookie,
      );

      expect(created.status).toBe(201);
      expect(response.status).toBe(200);
      expect(response.body).toEqual({
        ...(created.body as OrderResponse),
        status: 'Canceled',
      });
    });

    it('writes one OrderCanceled event for the first cancellation', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);
      const cancellationCountBefore = await countCanceledOrderEvents();

      const firstCancellation = await deleteOrder(
        (created.body as OrderResponse).id,
        sessionCookie,
      );
      const eventsAfterFirstCancellation = await canceledOrderEvents();
      const secondCancellation = await deleteOrder(
        (created.body as OrderResponse).id,
        sessionCookie,
      );
      const cancellationCountAfterRetry = await countCanceledOrderEvents();

      expect(firstCancellation.status).toBe(200);
      expect(eventsAfterFirstCancellation).toHaveLength(
        cancellationCountBefore + 1,
      );
      const eventRow =
        eventsAfterFirstCancellation[eventsAfterFirstCancellation.length - 1];
      expect(eventRow).toBeDefined();
      const event = fromBinary(OrderCanceledSchema, eventRow!.payload);
      expect(event).toMatchObject({
        eventId: eventRow!.event_id,
        orderId: (created.body as OrderResponse).id,
        ticket: { id: ticketId },
      });
      expect(secondCancellation.status).toBe(200);
      expect(cancellationCountAfterRetry).toBe(cancellationCountBefore + 1);
    });

    it('cancels an AwaitingPayment order belonging to the authenticated user', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);
      await setOrderState(
        (created.body as OrderResponse).id,
        'AwaitingPayment',
      );

      const response = await deleteOrder(
        (created.body as OrderResponse).id,
        sessionCookie,
      );

      expect(created.status).toBe(201);
      expect(response.status).toBe(200);
      expect(response.body).toEqual({
        ...(created.body as OrderResponse),
        status: 'Canceled',
      });
    });

    it('allows an owner to cancel an already canceled order again', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);
      const orderId = (created.body as OrderResponse).id;

      await deleteOrder(orderId, sessionCookie);
      const response = await deleteOrder(orderId, sessionCookie);

      expect(created.status).toBe(201);
      expect(response.status).toBe(200);
      expect(response.body).toEqual({
        ...(created.body as OrderResponse),
        status: 'Canceled',
      });
    });

    it('releases a canceled ticket for another user', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);

      const canceled = await deleteOrder(
        (created.body as OrderResponse).id,
        sessionCookie,
      );
      const replacement = await postOrder(
        { ticketId },
        anotherUser.sessionCookie,
      );

      expect(canceled.status).toBe(200);
      expect(replacement.status).toBe(201);
      expect(replacement.body).toEqual(
        expect.objectContaining({ ticketId, userId: anotherUser.userId }),
      );
    });

    it('returns 404 when the order does not exist', async () => {
      const response = await deleteOrder(randomUUID(), sessionCookie);

      expect(response.status).toBe(404);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'NOT_FOUND' })],
      });
    });

    it('returns 404 when another user owns the order', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);

      const response = await deleteOrder(
        (created.body as OrderResponse).id,
        anotherUser.sessionCookie,
      );

      expect(created.status).toBe(201);
      expect(response.status).toBe(404);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'NOT_FOUND' })],
      });
    });

    it('returns 409 when the order is complete', async () => {
      const ticketId = await seedProjectedTicket();
      const created = await postOrder({ ticketId }, sessionCookie);
      await setOrderState((created.body as OrderResponse).id, 'Complete');

      const response = await deleteOrder(
        (created.body as OrderResponse).id,
        sessionCookie,
      );

      expect(created.status).toBe(201);
      expect(response.status).toBe(409);
      expect(response.body).toEqual({
        errors: [expect.objectContaining({ code: 'ALREADY_EXISTS' })],
      });
    });

    it('rejects an invalid order ID', async () => {
      const response = await deleteOrder('not-a-uuid', sessionCookie);

      expect(response.status).toBe(400);
      expect(response.body).toEqual({
        errors: [
          expect.objectContaining({
            code: 'INVALID_ARGUMENT',
            field: 'orderId',
          }),
        ],
      });
    });
  });

  async function createAuthenticatedUser(): Promise<{
    sessionCookie: string;
    userId: string;
  }> {
    const email = `orders-${randomUUID()}@example.com`;
    const password = 'password123';
    const signupResponse = await postJson('/api/auth/signup', {
      email,
      name: 'Orders Integration User',
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

    return { sessionCookie: setCookie.split(';', 1)[0], userId: signup.userId };
  }

  async function setOrderState(
    orderId: string,
    status: 'AwaitingPayment' | 'Canceled' | 'Complete' | 'Created',
    expired = false,
  ): Promise<void> {
    const database = new Client({ connectionString: ordersDatabaseUrl });
    await database.connect();
    try {
      await database.query(
        `UPDATE orders
         SET status = $2,
             expires_at = CASE WHEN $3 THEN NOW() - INTERVAL '1 minute' ELSE expires_at END
         WHERE id = $1`,
        [orderId, status, expired],
      );
    } finally {
      await database.end();
    }
  }

  async function setOrderExpiration(
    orderId: string,
    offsetMilliseconds: number,
  ): Promise<void> {
    const database = new Client({ connectionString: ordersDatabaseUrl });
    await database.connect();
    try {
      await database.query('UPDATE orders SET expires_at = $2 WHERE id = $1', [
        orderId,
        new Date(Date.now() + offsetMilliseconds),
      ]);
    } finally {
      await database.end();
    }
  }

  async function seedProjectedTicket(): Promise<string> {
    const id = randomUUID();
    const database = new Client({ connectionString: ordersDatabaseUrl });
    await database.connect();
    try {
      await database.query(
        'INSERT INTO tickets (id, title, price) VALUES ($1, $2, $3)',
        [id, 'Projected concert ticket', 10_000],
      );
    } finally {
      await database.end();
    }
    return id;
  }

  async function canceledOrderEvents(): Promise<
    Array<{ event_id: string; payload: Buffer }>
  > {
    const database = new Client({ connectionString: ordersDatabaseUrl });
    await database.connect();
    try {
      const result = await database.query<{
        event_id: string;
        payload: Buffer;
      }>(
        `SELECT event_id::text, payload
         FROM outbox_events
         WHERE subject = 'orders.order.canceled.v1'
         ORDER BY created_at ASC, event_id ASC`,
      );
      return result.rows;
    } finally {
      await database.end();
    }
  }

  async function createdOrderEvents(): Promise<
    Array<{ event_id: string; payload: Buffer }>
  > {
    const database = new Client({ connectionString: ordersDatabaseUrl });
    await database.connect();
    try {
      const result = await database.query<{
        event_id: string;
        payload: Buffer;
      }>(
        `SELECT event_id::text, payload
         FROM outbox_events
         WHERE subject = 'orders.order.created.v1'
         ORDER BY created_at ASC, event_id ASC`,
      );
      return result.rows;
    } finally {
      await database.end();
    }
  }

  async function countCanceledOrderEvents(): Promise<number> {
    return (await canceledOrderEvents()).length;
  }

  async function countCreatedOrderEvents(): Promise<number> {
    return (await createdOrderEvents()).length;
  }

  function postOrder(
    body: { ticketId: string },
    cookie?: string,
  ): Promise<HttpResponse> {
    return postJson('/api/orders', body, cookie);
  }

  async function getOrders(cookie?: string): Promise<HttpResponse> {
    const response = await fetch(`${gatewayUrl}/api/orders`, {
      headers: cookie === undefined ? {} : { cookie },
    });
    return {
      body: await response.json(),
      headers: response.headers,
      status: response.status,
    };
  }

  async function getOrder(id: string, cookie?: string): Promise<HttpResponse> {
    const response = await fetch(`${gatewayUrl}/api/orders/${id}`, {
      headers: cookie === undefined ? {} : { cookie },
    });
    return {
      body: await response.json(),
      headers: response.headers,
      status: response.status,
    };
  }

  async function deleteOrder(
    id: string,
    cookie?: string,
  ): Promise<HttpResponse> {
    const response = await fetch(`${gatewayUrl}/api/orders/${id}`, {
      headers: cookie === undefined ? {} : { cookie },
      method: 'DELETE',
    });
    return {
      body: await response.json(),
      headers: response.headers,
      status: response.status,
    };
  }

  async function postJson(
    path: string,
    body: Record<string, string>,
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

interface AuthenticatedUser {
  sessionCookie: string;
  userId: string;
}

interface OrderResponse {
  expiresAt: string;
  id: string;
  status: string;
  ticketId: string;
  userId: string;
}

function runOrdersMigration(databaseUrl: string): Promise<void> {
  return runGoCommand(['run', './cmd/migrate'], { DATABASE_URL: databaseUrl });
}

function startOrdersService(
  grpcPort: number,
  databaseUrl: string,
): ChildProcessWithoutNullStreams {
  return spawn('go', ['run', './cmd/orders'], {
    cwd: ordersDirectory(),
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
    cwd: ordersDirectory(),
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
  ordersProcess: ChildProcessWithoutNullStreams,
): Promise<void> {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    if (ordersProcess.exitCode !== null) {
      throw new Error('Orders service exited before accepting gRPC requests.');
    }
    if (await canConnect(port)) {
      return;
    }
    await delay(100);
  }
  throw new Error('Orders service did not start within 15 seconds.');
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

async function stopOrdersService(
  ordersProcess: ChildProcessWithoutNullStreams | undefined,
): Promise<void> {
  if (!ordersProcess || ordersProcess.exitCode !== null) {
    return;
  }
  const exited = new Promise<void>((resolve) => {
    ordersProcess.once('exit', () => resolve());
  });
  terminateProcessGroup(ordersProcess, 'SIGTERM');
  const stopped = await Promise.race([
    exited.then(() => true),
    delay(5_000).then(() => false),
  ]);
  if (!stopped) {
    terminateProcessGroup(ordersProcess, 'SIGKILL');
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

function ordersDirectory(): string {
  return join(process.cwd(), 'apps/orders');
}
