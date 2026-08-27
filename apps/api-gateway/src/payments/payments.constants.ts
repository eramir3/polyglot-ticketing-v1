import type { Options } from '@grpc/proto-loader';

export const PAYMENTS_GRPC_CLIENT = 'PAYMENTS_GRPC_CLIENT';
export const PAYMENTS_GRPC_LOADER_OPTIONS = {
  enums: Number,
  longs: Number,
} satisfies Pick<Options, 'enums' | 'longs'>;
