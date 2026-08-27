import { Observable } from 'rxjs';

export interface CreatePaymentRequest {
  orderId: string;
  userId: string;
}

export interface PaymentGrpcResponse {
  id: string;
  orderId: string;
}

export interface CreatePaymentGrpcResponse {
  created: boolean;
  payment: PaymentGrpcResponse;
}

export interface PaymentResponse {
  id: string;
  orderId: string;
}

export interface PaymentCreationResult {
  created: boolean;
  payment: PaymentResponse;
}

export interface PaymentsGrpcService {
  createPayment(
    request: CreatePaymentRequest,
  ): Observable<CreatePaymentGrpcResponse>;
}
