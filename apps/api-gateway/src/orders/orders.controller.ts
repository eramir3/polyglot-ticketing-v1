import {
  Body,
  Controller,
  Get,
  HttpStatus,
  Post,
  Res,
  UseGuards,
} from '@nestjs/common';
import { CurrentUser } from '../identity/decorators/current-user.decorator';
import { AuthenticatedUser } from '../identity/authenticated-request';
import { SessionAuthGuard } from '../identity/guards/session-auth.guard';
import { CreateOrderDto } from './dtos/create-order.dto';
import { OrdersService } from './orders.service';
import { OrderResponse } from './orders.types';

@Controller('orders')
export class OrdersController {
  constructor(private readonly ordersService: OrdersService) {}

  @Post()
  @UseGuards(SessionAuthGuard)
  async createOrder(
    @Body() dto: CreateOrderDto,
    @CurrentUser() user: AuthenticatedUser,
    @Res({ passthrough: true }) response: StatusResponse,
  ): Promise<OrderResponse> {
    const result = await this.ordersService.createOrder(dto.ticketId, user.id);
    response.status(result.created ? HttpStatus.CREATED : HttpStatus.OK);

    return result.order;
  }

  @Get()
  @UseGuards(SessionAuthGuard)
  listOrders(@CurrentUser() user: AuthenticatedUser): Promise<OrderResponse[]> {
    return this.ordersService.listOrders(user.id);
  }
}

interface StatusResponse {
  status(code: number): StatusResponse;
}
