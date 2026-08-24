import { IsInt, IsNotEmpty, IsString, Min } from 'class-validator';

export class CreateTicketDto {
  @IsString({ message: 'Title must be a string' })
  @IsNotEmpty({ message: 'Title is required.' })
  title!: string;

  @IsInt({ message: 'Price must be an integer.' })
  @Min(1, { message: 'Price must be a positive integer.' })
  price!: number;
}
