import { join } from 'node:path';
import {
  Server,
  ServerCredentials,
  loadPackageDefinition,
} from '@grpc/grpc-js';
import { loadSync } from '@grpc/proto-loader';
import { ArgumentMetadata, ValidationPipe } from '@nestjs/common';
import {
  ClientProxyFactory,
  ClientGrpc,
  Transport,
} from '@nestjs/microservices';
import { createValidationApiError } from '../src/errors/validation-error';
import { AuthService } from '../src/identity/auth.service';
import {
  MAX_TICKET_PRICE,
  TICKETS_GRPC_LOADER_OPTIONS,
} from '../src/tickets/tickets.constants';
import { CreateTicketDto } from '../src/tickets/dtos/create-ticket.dto';
import { TicketsService } from '../src/tickets/tickets.service';

describe('tickets gRPC serialization', () => {
  let server: Server;
  let client: ClientGrpc & { close(): void };

  afterEach(async () => {
    client?.close();
    await stopServer(server);
  });

  it('deserializes an int64 price as a JSON number', async () => {
    const port = await startTicketsServer();
    client = ClientProxyFactory.create({
      transport: Transport.GRPC,
      options: {
        loader: {
          includeDirs: [join(process.cwd(), 'proto')],
          ...TICKETS_GRPC_LOADER_OPTIONS,
        },
        package: 'tickets.v1',
        protoPath: join(process.cwd(), 'proto/tickets/v1/tickets.proto'),
        url: `127.0.0.1:${port}`,
      },
    }) as ClientGrpc & { close(): void };
    const ticketsService = new TicketsService(
      {
        currentUser: async () => ({
          user: {
            email: 'test@example.com',
            emailVerified: true,
            id: 'user-1',
            name: 'Test User',
          },
        }),
      } as unknown as AuthService,
      client,
    );
    ticketsService.onModuleInit();

    await expect(
      ticketsService.createTicket({ price: 10_000, title: 'Metallica' }, {}),
    ).resolves.toEqual({
      id: 'ticket-1',
      price: 10_000,
      title: 'Metallica',
      userId: 'user-1',
    });
  });

  it.each([
    { price: 0, scenario: 'the price is zero' },
    {
      price: MAX_TICKET_PRICE + 1,
      scenario: 'the price exceeds the safe integer limit',
    },
  ])('rejects the request when $scenario', async ({ price }) => {
    const validationPipe = new ValidationPipe({
      exceptionFactory: createValidationApiError,
      transform: true,
      whitelist: true,
    });

    await expect(
      validationPipe.transform({ price, title: 'Metallica' }, {
        metatype: CreateTicketDto,
        type: 'body',
      } satisfies ArgumentMetadata),
    ).rejects.toMatchObject({
      response: {
        errors: [
          expect.objectContaining({ code: 'INVALID_PRICE', field: 'price' }),
        ],
      },
      status: 400,
    });
  });

  async function startTicketsServer(): Promise<number> {
    const definition = loadSync(
      join(process.cwd(), 'proto/tickets/v1/tickets.proto'),
      { includeDirs: [join(process.cwd(), 'proto')] },
    );
    const ticketPackage = loadPackageDefinition(definition) as unknown as {
      tickets: {
        v1: {
          TicketsService: { service: Parameters<Server['addService']>[0] };
        };
      };
    };

    server = new Server();
    server.addService(ticketPackage.tickets.v1.TicketsService.service, {
      createTicket: (
        _call: unknown,
        callback: (error: null, response: object) => void,
      ) => {
        callback(null, {
          id: 'ticket-1',
          price: 10_000,
          title: 'Metallica',
          userId: 'user-1',
        });
      },
    });

    return new Promise((resolve, reject) => {
      server.bindAsync(
        '127.0.0.1:0',
        ServerCredentials.createInsecure(),
        (error, port) => (error ? reject(error) : resolve(port)),
      );
    });
  }
});

function stopServer(server: Server | undefined): Promise<void> {
  if (!server) {
    return Promise.resolve();
  }

  return new Promise((resolve, reject) => {
    server.tryShutdown((error) => (error ? reject(error) : resolve()));
  });
}
