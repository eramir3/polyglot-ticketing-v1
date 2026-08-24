import type { Options } from '@grpc/proto-loader';

export const TICKETS_GRPC_CLIENT = 'TICKETS_GRPC_CLIENT';
export const MAX_TICKET_PRICE = Number.MAX_SAFE_INTEGER;
export const TICKETS_GRPC_LOADER_OPTIONS = {
  longs: Number,
} satisfies Pick<Options, 'longs'>;
