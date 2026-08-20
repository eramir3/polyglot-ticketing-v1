import { status } from '@grpc/grpc-js';
import { RpcException } from '@nestjs/microservices';

export interface ErrorItem {
  message: string;
  code: string;
  field?: string;
}

export class StructuredGrpcError extends RpcException {
  constructor(code: status, errors: ErrorItem[]) {
    super({
      code,
      details: JSON.stringify({ errors }),
    });
  }
}
