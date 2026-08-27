import { Inject, Injectable, OnModuleInit } from '@nestjs/common';
import type { ClientGrpc } from '@nestjs/microservices';
import { firstValueFrom } from 'rxjs';
import { ErrorCode, toPublicErrorCode } from '../errors/error-code';
import { throwGatewayGrpcError } from '../errors/throw-grpc-error';
import { PAYMENTS_GRPC_CLIENT } from './payments.constants';
import {
  PaymentCreationResult,
  PaymentResponse,
  PaymentsGrpcService,
} from './payments.types';

@Injectable()
export class PaymentsService implements OnModuleInit {
  private paymentsService!: PaymentsGrpcService;

  constructor(
    @Inject(PAYMENTS_GRPC_CLIENT) private readonly paymentsClient: ClientGrpc,
  ) {}

  onModuleInit(): void {
    this.paymentsService =
      this.paymentsClient.getService<PaymentsGrpcService>('PaymentsService');
  }

  async createPayment(
    orderId: string,
    userId: string,
  ): Promise<PaymentCreationResult> {
    try {
      const response = await firstValueFrom(
        this.paymentsService.createPayment({ orderId, userId }),
      );
      return {
        created: response.created,
        payment: toPaymentResponse(response.payment),
      };
    } catch (error: unknown) {
      throwGatewayGrpcError(error, {
        code: toPublicErrorCode(ErrorCode.INVALID_ARGUMENT),
        message: 'Order ID is invalid.',
      });
    }
  }
}

function toPaymentResponse(response: {
  id: string;
  orderId: string;
}): PaymentResponse {
  return { id: response.id, orderId: response.orderId };
}
