import { Inject, Injectable, OnModuleInit } from '@nestjs/common';
import type { ClientGrpc } from '@nestjs/microservices';
import { firstValueFrom } from 'rxjs';
import { OrderStatus as GrpcOrderStatus } from '../../../../protogen/ts/orders/v1/orders_pb.js';
import { ErrorCode, toPublicErrorCode } from '../errors/error-code';
import { throwGatewayGrpcError } from '../errors/throw-grpc-error';
import { ORDERS_GRPC_CLIENT } from './orders.constants';
import {
  CreateOrderGrpcResponse,
  ListOrdersGrpcResponse,
  OrderCreationResult,
  OrderGrpcResponse,
  OrderResponse,
  OrdersGrpcService,
  OrderStatus,
} from './orders.types';

@Injectable()
export class OrdersService implements OnModuleInit {
  private ordersService!: OrdersGrpcService;

  constructor(
    @Inject(ORDERS_GRPC_CLIENT) private readonly ordersClient: ClientGrpc,
  ) {}

  onModuleInit(): void {
    this.ordersService =
      this.ordersClient.getService<OrdersGrpcService>('OrdersService');
  }

  async createOrder(
    ticketId: string,
    userId: string,
  ): Promise<OrderCreationResult> {
    try {
      const response = await firstValueFrom(
        this.ordersService.createOrder({ ticketId, userId }),
      );
      return toCreateOrderResponse(response);
    } catch (error: unknown) {
      throwGatewayGrpcError(error, {
        code: toPublicErrorCode(ErrorCode.INVALID_ARGUMENT),
        message: 'Order data is invalid.',
      });
    }
  }

  async listOrders(userId: string): Promise<OrderResponse[]> {
    try {
      const response = await firstValueFrom(
        this.ordersService.listOrders({ userId }),
      );
      return toListOrdersResponse(response);
    } catch (error: unknown) {
      throwGatewayGrpcError(error, {
        code: toPublicErrorCode(ErrorCode.INTERNAL_ERROR),
        message: 'Unable to retrieve orders.',
      });
    }
  }
}

function toCreateOrderResponse(
  response: CreateOrderGrpcResponse,
): OrderCreationResult {
  return {
    created: response.created,
    order: toOrderResponse(response),
  };
}

function toListOrdersResponse(
  response: ListOrdersGrpcResponse,
): OrderResponse[] {
  return (response.orders ?? []).map(toOrderResponse);
}

function toOrderResponse(response: OrderGrpcResponse): OrderResponse {
  return {
    expiresAt: toDate(response.expiresAt).toISOString(),
    id: response.id,
    status: toOrderStatus(response.status),
    ticketId: response.ticketId,
    userId: response.userId,
  };
}

function toDate(timestamp: {
  seconds: string | number | bigint;
  nanos: number;
}): Date {
  return new Date(
    Number(timestamp.seconds) * 1_000 + timestamp.nanos / 1_000_000,
  );
}

function toOrderStatus(status: GrpcOrderStatus): OrderStatus {
  switch (status) {
    case GrpcOrderStatus.CREATED:
      return 'Created';
    case GrpcOrderStatus.CANCELED:
      return 'Canceled';
    case GrpcOrderStatus.AWAITING_PAYMENT:
      return 'AwaitingPayment';
    case GrpcOrderStatus.COMPLETE:
      return 'Complete';
    default:
      throw new Error(`Unsupported order status: ${status}`);
  }
}
