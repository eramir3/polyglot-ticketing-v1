import {
  Body,
  Controller,
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
import { CreateOrderResponse } from './orders.types';

@Controller('orders')
export class OrdersController {
  constructor(private readonly ordersService: OrdersService) {}

  @Post()
  @UseGuards(SessionAuthGuard)
  async createOrder(
    @Body() dto: CreateOrderDto,
    @CurrentUser() user: AuthenticatedUser,
    @Res({ passthrough: true }) response: StatusResponse,
  ): Promise<CreateOrderResponse> {
    const result = await this.ordersService.createOrder(dto.ticketId, user.id);
    response.status(result.created ? HttpStatus.CREATED : HttpStatus.OK);

    return result.order;
  }
}

interface StatusResponse {
  status(code: number): StatusResponse;
}
