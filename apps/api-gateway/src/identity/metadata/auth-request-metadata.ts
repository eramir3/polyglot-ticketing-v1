import { Metadata } from '@grpc/grpc-js';
import { IncomingHttpHeaders } from 'node:http';

/**
 * Converts selected HTTP request headers into gRPC metadata for Identity's
 * Better Auth request checks. This HTTP-to-gRPC boundary is intentionally an
 * allowlist: authentication credentials and arbitrary client headers must
 * remain at the API gateway.
 */
const FORWARDED_AUTH_HEADERS = [
  'origin',
  'referer',
  'sec-fetch-dest',
  'sec-fetch-mode',
  'sec-fetch-site',
  'user-agent',
] as const;

/**
 * Performs the HTTP-to-gRPC request-context conversion for the headers Better
 * Auth uses to validate trusted origins and browser provenance during sign-in.
 */
export function createAuthRequestMetadata(
  headers: IncomingHttpHeaders,
): Metadata {
  const metadata = new Metadata();

  for (const headerName of FORWARDED_AUTH_HEADERS) {
    const value = headers[headerName];
    if (typeof value === 'string') {
      metadata.set(headerName, value);
    }
  }

  return metadata;
}
