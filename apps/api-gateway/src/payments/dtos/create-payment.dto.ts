import { IsNotEmpty, IsUUID } from 'class-validator';

export class CreatePaymentDto {
  @IsNotEmpty({ message: 'Order ID is required.' })
  @IsUUID('4', { message: 'Order ID must be a valid UUID.' })
  orderId!: string;
}
