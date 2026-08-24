import { IncomingHttpHeaders } from 'node:http';
import { Body, Controller, Headers, Post } from '@nestjs/common';
import { CreateTicketDto } from './dtos/create-ticket.dto';
import { TicketsService } from './tickets.service';
import { CreateTicketResponse } from './tickets.types';

@Controller('tickets')
export class TicketsController {
  constructor(private readonly ticketsService: TicketsService) {}

  @Post()
  createTicket(
    @Body() dto: CreateTicketDto,
    @Headers() headers: IncomingHttpHeaders,
  ): Promise<CreateTicketResponse> {
    return this.ticketsService.createTicket(dto, headers);
  }
}
