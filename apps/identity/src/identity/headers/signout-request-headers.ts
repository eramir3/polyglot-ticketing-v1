import { Metadata } from '@grpc/grpc-js';

/**
 * Converts the cookie-only gRPC metadata sent by the API gateway into the
 * in-memory Web Headers object Better Auth requires for sign-out. Identity
 * remains gRPC-only; this does not create an HTTP request or route.
 */
export function createSignOutRequestHeaders(metadata: Metadata): Headers {
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
