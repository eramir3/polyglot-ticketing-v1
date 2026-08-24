import { Inject, Injectable, OnModuleInit } from '@nestjs/common';
import type { ClientGrpc } from '@nestjs/microservices';
import { firstValueFrom } from 'rxjs';
import { throwGatewayGrpcError } from '../errors/throw-grpc-error';
import { TICKETS_GRPC_CLIENT } from './tickets.constants';
import {
  CreateTicketRequest,
  CreateTicketResponse,
  Ticket,
  TicketsGrpcService,
  UpdateTicketRequest,
} from './tickets.types';

@Injectable()
export class TicketsService implements OnModuleInit {
  private ticketsService!: TicketsGrpcService;

  constructor(
    @Inject(TICKETS_GRPC_CLIENT) private readonly ticketsClient: ClientGrpc,
  ) {}

  onModuleInit(): void {
    this.ticketsService =
      this.ticketsClient.getService<TicketsGrpcService>('TicketsService');
  }

  async createTicket(
    request: Pick<CreateTicketRequest, 'price' | 'title'>,
    userId: string,
  ): Promise<CreateTicketResponse> {
    try {
      return await firstValueFrom(
        this.ticketsService.createTicket({
          ...request,
          userId,
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

  async updateTicket(
    id: string,
    request: Pick<UpdateTicketRequest, 'price' | 'title'>,
    userId: string,
  ): Promise<Ticket> {
    try {
      const response = await firstValueFrom(
        this.ticketsService.updateTicket({
          ...request,
          id,
          userId,
        }),
      );
      return response.ticket;
    } catch (error: unknown) {
      throwGatewayGrpcError(error, {
        code: 'INVALID_ARGUMENT',
        message: 'Ticket data is invalid.',
      });
    }
  }

  async getTicket(id: string): Promise<Ticket> {
    try {
      const response = await firstValueFrom(
        this.ticketsService.getTicket({ id }),
      );
      return response.ticket;
    } catch (error: unknown) {
      throwGatewayGrpcError(error);
    }
  }
}
