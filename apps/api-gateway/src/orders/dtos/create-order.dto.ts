import { IsUUID } from 'class-validator';

export class CreateOrderDto {
  @IsUUID('4', { message: 'Ticket ID must be a valid UUID.' })
  ticketId!: string;
}
