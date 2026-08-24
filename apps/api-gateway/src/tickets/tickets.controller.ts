import {
  Body,
  Controller,
  Get,
  Param,
  Post,
  Put,
  UseGuards,
} from '@nestjs/common';
import { CurrentUser } from '../identity/decorators/current-user.decorator';
import { AuthenticatedUser } from '../identity/authenticated-request';
import { SessionAuthGuard } from '../identity/guards/session-auth.guard';
import { CreateTicketDto } from './dtos/create-ticket.dto';
import { UpdateTicketDto } from './dtos/update-ticket.dto';
import { TicketsService } from './tickets.service';
import { CreateTicketResponse, Ticket } from './tickets.types';

@Controller('tickets')
export class TicketsController {
  constructor(private readonly ticketsService: TicketsService) {}

  @Get()
  listTickets(): Promise<Ticket[]> {
    return this.ticketsService.listTickets();
  }

  @Get(':id')
  getTicket(@Param('id') id: string): Promise<Ticket> {
    return this.ticketsService.getTicket(id);
  }

  @Put(':id')
  @UseGuards(SessionAuthGuard)
  updateTicket(
    @Param('id') id: string,
    @Body() dto: UpdateTicketDto,
    @CurrentUser() user: AuthenticatedUser,
  ): Promise<Ticket> {
    return this.ticketsService.updateTicket(id, dto, user.id);
  }

  @Post()
  @UseGuards(SessionAuthGuard)
  createTicket(
    @Body() dto: CreateTicketDto,
    @CurrentUser() user: AuthenticatedUser,
  ): Promise<CreateTicketResponse> {
    return this.ticketsService.createTicket(dto, user.id);
  }
}
