import type { Options } from '@grpc/proto-loader';

export const ORDERS_GRPC_CLIENT = 'ORDERS_GRPC_CLIENT';
export const ORDERS_GRPC_LOADER_OPTIONS = {
  enums: Number,
  longs: Number,
} satisfies Pick<Options, 'enums' | 'longs'>;
