import { IsInt, IsNotEmpty, IsString, Max, Min } from 'class-validator';
import { MAX_TICKET_PRICE } from '../tickets.constants';

export class CreateTicketDto {
  @IsString({ message: 'Title must be a string' })
  @IsNotEmpty({ message: 'Title is required.' })
  title!: string;

  @IsInt({ message: 'Price must be an integer.' })
  @Min(1, { message: 'Price must be a positive integer.' })
  @Max(MAX_TICKET_PRICE, {
    message: 'Price exceeds the maximum supported amount.',
  })
  price!: number;
}
