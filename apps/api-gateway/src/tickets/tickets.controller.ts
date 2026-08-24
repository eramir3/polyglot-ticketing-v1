import { IncomingHttpHeaders } from 'node:http';
import { Body, Controller, Get, Headers, Post } from '@nestjs/common';
import { CreateTicketDto } from './dtos/create-ticket.dto';
import { TicketsService } from './tickets.service';
import { CreateTicketResponse, Ticket } from './tickets.types';

@Controller('tickets')
export class TicketsController {
  constructor(private readonly ticketsService: TicketsService) {}

  @Get()
  listTickets(): Promise<Ticket[]> {
    return this.ticketsService.listTickets();
  }

  @Post()
  createTicket(
    @Body() dto: CreateTicketDto,
    @Headers() headers: IncomingHttpHeaders,
  ): Promise<CreateTicketResponse> {
    return this.ticketsService.createTicket(dto, headers);
  }
}
