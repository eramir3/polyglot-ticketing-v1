import { OrderStatus as GrpcOrderStatus } from '../../../../protogen/ts/orders/v1/orders_pb.js';
import { Observable } from 'rxjs';

export interface Timestamp {
  seconds: string | number | bigint;
  nanos: number;
}

export type OrderStatus =
  | 'AwaitingPayment'
  | 'Canceled'
  | 'Complete'
  | 'Created';

export interface CreateOrderRequest {
  ticketId: string;
  userId: string;
}

export interface CreateOrderGrpcResponse {
  created: boolean;
  expiresAt: Timestamp;
  id: string;
  status: GrpcOrderStatus;
  ticketId: string;
  userId: string;
}

export interface ListOrdersRequest {
  userId: string;
}

export interface ListOrdersGrpcResponse {
  orders?: OrderGrpcResponse[];
}

export interface OrderGrpcResponse {
  expiresAt: Timestamp;
  id: string;
  status: GrpcOrderStatus;
  ticketId: string;
  userId: string;
}

export interface OrderResponse {
  expiresAt: string;
  id: string;
  status: OrderStatus;
  ticketId: string;
  userId: string;
}

export interface OrderCreationResult {
  created: boolean;
  order: OrderResponse;
}

export interface OrdersGrpcService {
  createOrder(request: CreateOrderRequest): Observable<CreateOrderGrpcResponse>;
  listOrders(request: ListOrdersRequest): Observable<ListOrdersGrpcResponse>;
}
