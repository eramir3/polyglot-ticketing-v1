import { IncomingHttpHeaders } from 'node:http';
import { Inject, Injectable, OnModuleInit } from '@nestjs/common';
import type { ClientGrpc } from '@nestjs/microservices';
import { firstValueFrom } from 'rxjs';
import { throwGatewayGrpcError } from '../errors/throw-grpc-error';
import { AuthService } from '../identity/auth.service';
import { TICKETS_GRPC_CLIENT } from './tickets.constants';
import {
  CreateTicketRequest,
  CreateTicketResponse,
  Ticket,
  TicketsGrpcService,
} from './tickets.types';

@Injectable()
export class TicketsService implements OnModuleInit {
  private ticketsService!: TicketsGrpcService;

  constructor(
    private readonly authService: AuthService,
    @Inject(TICKETS_GRPC_CLIENT) private readonly ticketsClient: ClientGrpc,
  ) {}

  onModuleInit(): void {
    this.ticketsService =
      this.ticketsClient.getService<TicketsGrpcService>('TicketsService');
  }

  async createTicket(
    request: Pick<CreateTicketRequest, 'price' | 'title'>,
    headers: IncomingHttpHeaders,
  ): Promise<CreateTicketResponse> {
    const currentUser = await this.authService.currentUser(headers);

    try {
      return await firstValueFrom(
        this.ticketsService.createTicket({
          ...request,
          userId: currentUser.user.id,
        }),
      );
    } catch (error: unknown) {
      throwGatewayGrpcError(error, {
        code: 'INVALID_ARGUMENT',
        message: 'Ticket data is invalid.',
      });
    }
  }

  async listTickets(): Promise<Ticket[]> {
    try {
      const response = await firstValueFrom(
        this.ticketsService.listTickets({}),
      );
      return response.tickets;
    } catch (error: unknown) {
      throwGatewayGrpcError(error);
    }
  }
}
