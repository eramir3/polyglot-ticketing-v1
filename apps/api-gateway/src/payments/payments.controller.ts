import {
  Body,
  Controller,
  HttpStatus,
  Post,
  Res,
  UseGuards,
} from '@nestjs/common';
import { AuthenticatedUser } from '../identity/authenticated-request';
import { CurrentUser } from '../identity/decorators/current-user.decorator';
import { SessionAuthGuard } from '../identity/guards/session-auth.guard';
import { CreatePaymentDto } from './dtos/create-payment.dto';
import { PaymentsService } from './payments.service';
import { PaymentResponse } from './payments.types';

@Controller('payments')
export class PaymentsController {
  constructor(private readonly paymentsService: PaymentsService) {}

  @Post()
  @UseGuards(SessionAuthGuard)
  async createPayment(
    @Body() dto: CreatePaymentDto,
    @CurrentUser() user: AuthenticatedUser,
    @Res({ passthrough: true }) response: StatusResponse,
  ): Promise<PaymentResponse> {
    const result = await this.paymentsService.createPayment(
      dto.orderId,
      user.id,
    );
    response.status(result.created ? HttpStatus.CREATED : HttpStatus.OK);
    return result.payment;
  }
}

interface StatusResponse {
  status(code: number): StatusResponse;
}
