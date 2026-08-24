import { Metadata } from '@grpc/grpc-js';

/**
 * Converts cookie-only gRPC metadata from the API gateway into the in-memory
 * Web Headers object Better Auth uses to read the current session.
 */
export function createSessionRequestHeaders(metadata: Metadata): Headers {
  const headers = new Headers();
  const cookie = metadata.get('cookie').find(isString);

  if (cookie !== undefined) {
    headers.set('cookie', cookie);
  }

  return headers;
}

function isString(value: string | Buffer): value is string {
  return typeof value === 'string';
}
