import { Body, Controller, Post, UseGuards } from '@nestjs/common';
import { CurrentUser } from '../identity/decorators/current-user.decorator';
import { AuthenticatedUser } from '../identity/authenticated-request';
import { SessionAuthGuard } from '../identity/guards/session-auth.guard';
import { CreateOrderDto } from './dtos/create-order.dto';
import { OrdersService } from './orders.service';
import { CreateOrderResponse } from './orders.types';

@Controller('orders')
export class OrdersController {
  constructor(private readonly ordersService: OrdersService) {}

  @Post()
  @UseGuards(SessionAuthGuard)
  createOrder(
    @Body() dto: CreateOrderDto,
    @CurrentUser() user: AuthenticatedUser,
  ): Promise<CreateOrderResponse> {
    return this.ordersService.createOrder(dto.ticketId, user.id);
  }
}
